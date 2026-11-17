$ErrorActionPreference = 'Stop'
go build -ldflags '-H=windowsgui' -o reia-service.exe ./cmd/reia-service
if ($LASTEXITCODE -ne 0) { throw 'service build failed' }
go build -o reia-agent.exe ./cmd/reia-agent
if ($LASTEXITCODE -ne 0) { throw 'agent build failed' }
go build -o reia-tray.exe ./cmd/reia-tray
if ($LASTEXITCODE -ne 0) { throw 'tray build failed' }
go build -ldflags '-H=windowsgui' -o reia.exe ./cmd/reia
if ($LASTEXITCODE -ne 0) { throw 'launcher build failed' }
