# Nautilus

[English](README.md) | 繁體中文

Nautilus 是一個動態服務管理與代理系統，專為高可用性的請求路由與服務發現而設計。透過 Unix Domain Sockets (UDS) 與設定熱重載，實現流暢的流量管理。

## 核心功能

- **設定熱重載**：自動追蹤 `.ntl` 或 `Ntlfile` 設定檔的變更。
- **動態服務發現**：即時的服務註冊表管理。
- **UDS 代理**：透過 Unix Domain Sockets 高效轉發請求。
- **優雅的生命週期管理**：自動化的 Socket 監聽器與服務狀態清理。

## 開始使用

### 先決條件

- Go 1.25.6 或更高版本。

### 安裝

```bash
# Clone 儲存庫
git clone https://github.com/your-repo/nautilus.git
cd nautilus

# 編譯核心執行檔
go build -o bin/nautilus-core ./cmd/nautilus-core
```

### 使用方式

執行 Nautilus 核心服務：

```bash
./bin/nautilus-core --config=my-app.ntl
```

## 設定

Nautilus 使用 `Ntlfile` 作為設定檔，經由 `ntlc` 編譯器轉換為 binary 格式 (`.ntl`) 以供核心引擎進行熱重載。

### 設定檔編譯 (ntlc)

使用 `ntlc` 工具將 `Ntlfile` 編譯為 Nautilus 核心可讀取的 binary 格式 (`.ntl`)：

```bash
# 基本編譯指令
./bin/ntlc -i Ntlfile -o nautilus.ntl
```

### 設定範例 (Ntlfile)

```text
# 基礎路由規則
GET /api/v1/users $user-service
    $SetHeader(X-Source, Nautilus)
    $BasicAuth(admin, secret)

POST /upload/* storage-service
    $IPAllow(192.168.1.0/24)
```

詳細的語法規格、內建中間件與虛擬服務清單，請參閱 [Configuration Guide](./docs/configuration.md)。

## 架構

- **`cmd/`**: 核心引擎與 `ntlc` 編譯器的入口程式。
- **`internal/`**: 包含代理、服務註冊與設定監控的核心邏輯。

## 授權

本專案依照 LICENSE 檔案中的條款授權。
