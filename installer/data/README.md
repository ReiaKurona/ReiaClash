把预制的 Geo 文件放在此目录，和 reia-agent、reia-tray、mihomo 二进制一起分发。

常用文件名：geoip.dat、geosite.dat、country.mmdb、GeoLite2-ASN.mmdb。
具体需要哪些文件取决于订阅的 geodata-mode、DNS 和规则配置，文件格式必须匹配。
启动、配置校验和特权服务统一使用这个 data 目录，不依赖启动时的工作目录。
Linux/macOS 下当前登录用户必须能写入这个目录。
