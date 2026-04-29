# Nautilus

專為物理隔離與邊緣運算設計的高性能 UDS 路由引擎

Nautilus 是一個次世代路由架構，旨在透過 Unix Domain Sockets (UDS) 取代傳統的 TCP/IP 網路棧。它為高安全性環境與資源受限的邊緣節點提供了一個不可掃描、零延遲的通訊基礎設施。

## 為什麼選擇 Nautilus？

*   不可掃描性：Nautilus 僅監聽 UDS .sock 檔案，傳統的網路埠位掃描工具完全無法偵測，極大地縮小了受攻擊面。
*   零延遲架構：繞過 TCP 棧減少了 CPU 開銷與上下文切換，為進程間通訊（IPC）提供接近硬體極限的效能。
*   檔案系統即註冊表：無需 Etcd 或 Consul。服務發現直接由主機檔案系統的目錄結構自動處理。
*   編譯型路由：在 Ntlfile 中定義的規則會被編譯成優化的 Radix Tree 二進位檔，支援原子化、零停機的配置更新。

## 核心組件

*   Nautilus Core (Go)：數據面轉發引擎，具備原子化熱更新與 Round-robin 負載平衡功能。
*   ntlc (Go)：路由編譯器，將 DSL 規則轉換為高性能二進位快照。
*   Relay Sidecar (Rust)：基於非同步架構的適配器，負責將 UDS 流量轉譯至現有的 TCP 服務，並具備主動健康檢查機制。

## 快速開始

### 1. 定義路由規則 (Ntlfile)
```nautifile
# 將 localhost 流量導向 nginx 服務
GET localhost/api/[v1|v2] nginx
    $BasicAuth("admin", "secret")
    $Log("API_ACCESS")

# 捕捉所有請求的虛擬服務
$ok("Nautilus 運行中")
```

### 2. 編譯並執行
```bash
# 編譯路由快照
ntlc -i Ntlfile -o nautilus.ntl

# 啟動核心引擎
nautilus-core --config nautilus.ntl
```

## 技術文件

*   [架構設計 (English)](docs/ARCHITECTURE.md) - 深入了解 Radix Tree 與 UDS 設計。
*   [Ntlfile 語法規範 (English)](docs/NTLFILE_SPEC.md) - 完整的語法與中間件指南。
*   [開發者手冊 (English)](docs/DEVELOPMENT.md) - 環境建置、測試與貢獻指南。

## 整合測試

Nautilus 提供了一套強大的 Docker 整合測試工具，支援原始碼即時編譯：

```bash
# 執行完整整合測試
.\test\run_tests.bat  # Windows
bash ./test/run_tests.sh  # Linux/macOS
```

## 授權協議

本專案採用 MIT 授權協議 - 詳見 LICENSE 檔案。
