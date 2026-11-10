use axum::{
    body::Body,
    http::{header, HeaderValue, Method, Request, Response, StatusCode},
};
use std::future::Future;
use std::pin::Pin;
use std::task::{Context, Poll};
use tower::{Layer, Service};

/// 自定义 Axum 中间件层：专门处理 Chrome PNA 授权与 CORS 动态回显
#[derive(Clone)]
pub struct PnaCorsLayer;

impl<S> Layer<S> for PnaCorsLayer {
    type Service = PnaCorsMiddleware<S>;

    fn layer(&self, inner: S) -> Self::Service {
        PnaCorsMiddleware { inner }
    }
}

/// 实际拦截处理请求与响应的中间件服务
#[derive(Clone)]
pub struct PnaCorsMiddleware<S> {
    inner: S,
}

impl<S> Service<Request<Body>> for PnaCorsMiddleware<S>
where
    S: Service<Request<Body>, Response = Response<Body>> + Clone + Send + 'static,
    S::Future: Send + 'static,
{
    type Response = Response<Body>;
    type Error = S::Error;
    type Future = Pin<Box<dyn Future<Output = Result<Self::Response, Self::Error>> + Send>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.inner.poll_ready(cx)
    }

    fn call(&mut self, req: Request<Body>) -> Self::Future {
        let mut inner = self.inner.clone();

        // 获取请求来源的 Origin，若没有则回退为通配符 "*"
        let origin = req
            .headers()
            .get(header::ORIGIN)
            .cloned()
            .unwrap_or_else(|| HeaderValue::from_static("*"));

        // 如果是浏览器发起的 OPTIONS 预检请求，直接就地快速放行并注入 PNA 许可头
        if req.method() == Method::OPTIONS {
            return Box::pin(async move {
                let mut res = Response::builder()
                    .status(StatusCode::NO_CONTENT)
                    .body(Body::empty())
                    .unwrap();

                let headers = res.headers_mut();
                headers.insert(header::ACCESS_CONTROL_ALLOW_ORIGIN, origin);
                headers.insert(
                    header::ACCESS_CONTROL_ALLOW_METHODS,
                    HeaderValue::from_static("GET, POST, PUT, DELETE, OPTIONS, PATCH"),
                );
                headers.insert(
                    header::ACCESS_CONTROL_ALLOW_HEADERS,
                    HeaderValue::from_static("*"),
                );
                // 关键点：授予 Chrome 本地私有网络访问权
                headers.insert(
                    header::HeaderName::from_static("access-control-allow-private-network"),
                    HeaderValue::from_static("true"),
                );

                Ok(res)
            });
        }

        // 如果是正常的业务请求，继续传给业务处理函数，并在结果返回时追加对应响应头
        Box::pin(async move {
            let mut res = inner.call(req).await?;
            let headers = res.headers_mut();
            headers.insert(header::ACCESS_CONTROL_ALLOW_ORIGIN, origin);
            headers.insert(
                header::HeaderName::from_static("access-control-allow-private-network"),
                HeaderValue::from_static("true"),
            );
            Ok(res)
        })
    }
}