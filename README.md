# IM(客服系統)

即時客服系統的示範專案，提供玩家端與客服後台的互動範本。整體功能包含：

- Golang 實作的 WebSocket 聊天與後端 API，採用類似 WuKongIM 的事件格式。
- 使用 MySQL 永久保存多代理的管理員、客服與玩家帳號資料，並透過 Redis 儲存 JWT token 會話狀態。
- 使用 Pure Admin 視覺語彙打造的客服後台控制台，內含即時指標、抽屜式概況與房間清單。
- 參考 WuKongIM 介面重構的玩家端聊天體驗，具備序號標記與輸入中回報。
- WebSocket 協定與聊天室內存資料結構，提供序號、同步歷史與指派客服流程。

## 專案結構

```
.
├── cmd/server          # HTTP / WebSocket 伺服器進入點
├── internal            # 伺服器核心邏輯
│   ├── server          # HTTP handler 與路由
│   └── ws              # WebSocket hub、房間與訊息處理
├── web                 # 前端靜態資源
│   ├── admin           # 客服後台（Pure Admin 版型）
│   ├── client          # 玩家聊天視窗（微信風格）
│   └── static          # 共用 CSS 與 JS
└── README.md
```

## 系統需求與設定

- **MySQL**：儲存管理員、客服與玩家的帳號資料，以及各代理的 API 設定。
- **Redis**：保存登入後產生的 JWT token，作為會話有效性的檢查來源。
- **設定檔 `setting.conf`**：位於專案根目錄，可調整 MySQL、Redis 與 JWT 相關資訊；伺服器啟動時會依據此設定建立連線並自動初始化所需資料表。

`setting.conf` 範例：

```ini
[mysql]
host=127.0.0.1
port=3306
user=root
password=
database=im

[redis]
addr=127.0.0.1:6379
password=
db=0

[jwt]
secret=change-me
issuer=im-system
expiry=86400
```

> 提示：首次啟動會自動建立 `accounts` 與 `agency_settings` 資料表，並在缺少時預置 `admin01 / admin01pass` 管理員帳號。

## 啟動方式

1. 確認設定檔中的 MySQL 與 Redis 已可連線。

2. 安裝套件並建置：

   ```bash
   go mod tidy
   ```

3. 啟動伺服器：

   ```bash
   go run ./cmd/server
   ```

   伺服器預設執行於 <http://localhost:8080>。

3. 前端頁面：

   - 客服後台：<http://localhost:8080/admin/>
   - 玩家端：<http://localhost:8080/client/>

## WebSocket 協定

所有訊息以 JSON 格式傳輸，並帶有 `cmd` 與 `type` 兩個欄位以兼容舊版協定：

```json
{
  "cmd": "chat.message",
  "type": "chat.message",
  "roomId": "房間 ID",
  "senderId": "發送者 ID",
  "senderRole": "player | agent",
  "displayName": "顯示名稱",
  "content": "訊息內容",
  "timestamp": "RFC3339 時間戳",
  "seq": 12,
  "ack": 12,
  "metadata": { "status": "typing" },
  "payload": { "nextSeq": 12 },
  "history": [ ... ]
}
```

事件類型採用 WuKongIM 類似語彙：

- `chat.message`：聊天訊息，伺服器會帶上 `seq/ack` 以利客戶端對齊。
- `chat.typing`：輸入中提示，只即時廣播不寫入歷史，`ack` 表示最新序號。
- `chat.history`：同步歷史，`history` 欄位為訊息陣列，`payload.nextSeq` 為下一個序號。
- `system.notice`：系統提示（加入、離線、指派客服），可能包含額外 `metadata`。 

## REST API

| Method | Path                                      | 說明                       |
| ------ | ----------------------------------------- | -------------------------- |
| GET    | `/api/rooms`                              | 取得所有房間摘要           |
| GET    | `/api/rooms/{roomId}`                     | 取得指定房間詳情（含歷史） |
| GET    | `/api/rooms/{roomId}/messages?since={n}`  | 依序號增量拉取聊天歷史     |
| POST   | `/api/rooms/{roomId}/assign`              | 指派客服至指定房間         |
| GET    | `/api/agencies/settings`                  | 取得所有代理的 API 設定（需管理員） |
| POST   | `/api/agencies/settings/{agency}`         | 新增或更新指定代理的 API 設定 |

## 測試

專案包含針對 WebSocket hub 的單元測試，可透過以下指令執行：

```bash
go test ./...
```

## 授權

此範例僅供教學與內部驗證使用。
