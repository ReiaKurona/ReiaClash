/** Browser-only protocol adapter; import is safe during SSR, instantiate after mount. */
export class ReiaError extends Error {
  constructor(message, { code = 'request_failed', status = 0, uncertain = false, cause } = {}) {
    super(message);
    this.name = 'ReiaError';
    Object.assign(this, { code, status, uncertain, cause });
  }
}

export class ReiaClient {
  constructor({ agentBase = 'http://127.0.0.1:25500', fetchImpl = globalThis.fetch?.bind(globalThis),
    getPanelURL = () => globalThis.location.href } = {}) {
    const base = new URL(agentBase);
    if (base.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(base.hostname) ||
        base.port !== '25500' || base.username || base.password || base.pathname !== '/' || base.search || base.hash) {
      throw new ReiaError('Agent 地址必须是本机 25500 端口', { code: 'invalid_agent_base' });
    }
    if (typeof fetchImpl !== 'function') throw new ReiaError('当前环境没有 fetch', { code: 'unsupported_environment' });
    this.agentBase = base.origin;
    this.fetchImpl = fetchImpl;
    this.getPanelURL = getPanelURL;
    this.session = null;
    this.mutationPending = false;
    this.generation = 0;
    this.stopDiscovery = null;
  }

  get connected() { return this.session !== null; }
  get controller() { return this.session?.mihomo_controller ?? null; }

  async _request(base, path, { method = 'GET', body, signal, timeoutMs, headers = {}, mutation = method !== 'GET' } = {}) {
    const target = new URL(path, base + '/');
    if (!path.startsWith('/') || target.origin !== base) throw new ReiaError('接口路径必须在本机接口内', { code: 'invalid_path' });
    if (mutation && this.mutationPending) throw new ReiaError('上一项操作尚未完成', { code: 'operation_in_progress' });
    if (mutation) this.mutationPending = true;
    const abort = new AbortController();
    const cancel = () => abort.abort();
    signal?.addEventListener('abort', cancel, { once: true });
    if (signal?.aborted) cancel();
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; abort.abort(); }, timeoutMs ?? (mutation ? 300000 : 10000));
    let sent = false;
    try {
      if (abort.signal.aborted) throw new ReiaError('操作已取消', { code: 'aborted' });
      sent = true;
      const response = await this.fetchImpl(target.href, {
        method, headers: body === undefined ? headers : { ...headers, 'Content-Type': 'application/json' },
        body: body === undefined ? undefined : JSON.stringify(body), signal: abort.signal,
        mode: 'cors', credentials: 'omit', cache: 'no-store', redirect: 'error', targetAddressSpace: 'loopback',
      });
      const text = await response.text();
      let data = null;
      try { data = text ? JSON.parse(text) : null; } catch {
        if (response.ok) throw new ReiaError('接口返回了非 JSON 内容', { code: 'invalid_response', status: response.status, uncertain: mutation });
      }
      if (!response.ok || data?.success === false) throw new ReiaError(data?.message || `接口请求失败 (${response.status})`, {
        code: data?.code || 'http_error', status: response.status,
      });
      return data;
    } catch (error) {
      if (error instanceof ReiaError) throw error;
      throw new ReiaError(timedOut ? '操作超时，请查询实际状态后再决定是否重试' :
        abort.signal.aborted ? '操作已取消' : '无法访问本机接口，请检查客户端、浏览器权限及站点策略', {
        code: timedOut ? 'timeout' : abort.signal.aborted ? 'aborted' : 'network_error',
        uncertain: mutation && sent, cause: error,
      });
    } finally {
      clearTimeout(timer);
      signal?.removeEventListener('abort', cancel);
      if (mutation) this.mutationPending = false;
    }
  }

  agent(path, options) { return this._request(this.agentBase, path, options); }

  async connect({ signal, timeoutMs = 5000 } = {}) {
    const generation = ++this.generation;
    try {
      const page = new URL(this.getPanelURL());
      const session = await this.agent('/api/handshake', {
        method: 'POST', mutation: false, signal, timeoutMs,
        body: { panel_origin: page.origin, panel_url: page.href },
      });
      const controller = session?.mihomo_controller;
      if (!controller?.secret || controller.api_base !== 'http://127.0.0.1:9090') {
        throw new ReiaError('握手未返回有效的本机控制器', { code: 'invalid_handshake' });
      }
      if (generation !== this.generation || signal?.aborted) throw new ReiaError('本次连接已失效', { code: 'aborted' });
      this.session = session;
      return session;
    } catch (error) {
      if (generation === this.generation) this.session = null;
      throw error;
    }
  }

  disconnect() { ++this.generation; this.session = null; }

  /** Repeats only the idempotent handshake; mutations are never replayed. */
  startDiscovery(onChange = () => {}, { intervalMs = 5000 } = {}) {
    this.stopDiscovery?.();
    const abort = new AbortController();
    let timer;
    const tick = async () => {
      if (abort.signal.aborted) return;
      let result;
      const pending = this.connect({ signal: abort.signal });
      const generation = this.generation;
      try { await pending; result = { connected: true, error: null }; }
      catch (error) { result = { connected: false, error }; }
      if (!abort.signal.aborted && generation === this.generation) {
        try { onChange(result); } finally { if (!abort.signal.aborted) timer = setTimeout(tick, intervalMs); }
      } else if (!abort.signal.aborted) timer = setTimeout(tick, intervalMs);
    };
    const stop = () => {
      abort.abort(); clearTimeout(timer); this.disconnect();
      if (this.stopDiscovery === stop) this.stopDiscovery = null;
    };
    this.stopDiscovery = stop;
    void tick();
    return stop;
  }

  core(path, options = {}) {
    const controller = this.controller;
    if (!controller) return Promise.reject(new ReiaError('请先连接本机客户端', { code: 'not_connected' }));
    return this._request(controller.api_base, path, { ...options,
      headers: { ...options.headers, Authorization: `Bearer ${controller.secret}` } });
  }

  health(options) { return this.agent('/api/health', options); }
  status(options) { return this.agent('/api/agent/real-status', options); }
  outboundIP(options) { return this.agent('/api/agent/outbound-ip', options); }
  systemStatus(options) { return this.agent('/api/system/status', options); }
  panel(options) { return this.agent('/api/agent/panel', options); }
  events(since = 0, options) { return this.agent('/api/agent/events?since=' + encodeURIComponent(since), options); }
  setEnabled(enable, options) { return this.agent('/api/agent/master', { ...options, method: 'POST', body: { enable } }); }
  settings(patch, options) { return this.agent('/api/agent/settings', { ...options, method: 'POST', body: patch }); }
  setAutostart(enable, options) { return this.agent('/api/system/autostart', { ...options, method: 'POST', body: { enable } }); }
  setService(enable, { repair = false, ...options } = {}) {
    return this.agent('/api/system/core-service', { ...options, method: 'POST', body: { enable, repair } });
  }
  geoLowMemory(options) { return this.agent('/api/agent/geo-low-memory', options); }
  setGeoLowMemory(enable, options) { return this.agent('/api/agent/geo-low-memory', { ...options, method: 'POST', body: { enable } }); }
  proxies(options) { return this.agent('/api/core/proxies', options); }
  select(group, node, options) { return this.agent('/api/core/select', { ...options, method: 'POST', body: { group, node } }); }
  delay(group, options) { return this.agent('/api/core/group-delay', { ...options, method: 'POST', body: { group } }); }
  importSubscription(url, options) { return this.agent('/api/subscription/url', { ...options, method: 'POST', body: { url } }); }
  subscriptionStatus(options) { return this.agent('/api/subscription/status', options); }
  refreshSubscription(options) { return this.agent('/api/subscription/refresh', { ...options, method: 'POST' }); }
  connections(options) { return this.agent('/api/core/connections', options); }
  closeConnection(id, options) {
    if (!id) return Promise.reject(new ReiaError('连接 ID 不能为空；关闭全部请使用 closeConnections()', { code: 'invalid_id' }));
    return this.agent('/api/core/connections/' + encodeURIComponent(id), { ...options, method: 'DELETE' });
  }
  closeConnections(options) { return this.agent('/api/core/connections', { ...options, method: 'DELETE' }); }
  dnsQuery(name, type = 'A', options) { return this.agent('/api/core/dns/query?' + new URLSearchParams({ name, type }), options); }
  dnsRecords(options) { return this.agent('/api/core/dns/records', options); }
  clearDnsRecords(options) { return this.agent('/api/core/dns/records', { ...options, method: 'DELETE' }); }
  restart(options) { return this.agent('/api/core/restart', { ...options, method: 'POST' }); }
  checkUpdate(options) { return this.agent('/api/agent/check-update', options); }
  upgrade(is_silent = true, options) { return this.agent('/api/agent/upgrade', { ...options, method: 'POST', body: { is_silent } }); }
  exitAgent(options) { return this.agent('/api/agent/exit', { timeoutMs: 10000, ...options, method: 'POST' }); }
  clearAgentCache(options) { return this.agent('/api/agent/cache/clear', { ...options, method: 'POST' }); }
  clearAgentLogs(options) { return this.agent('/api/agent/logs/clear', { ...options, method: 'POST' }); }
  openDirectory(directory, options) { return this.agent('/api/agent/directory/open', { ...options, method: 'POST', body: { directory } }); }
}
