# Asynq Worker Service 🚀

Worker service berbasis Go (Golang) dan [Asynq](https://github.com/hibiken/asynq) untuk memproses *asynchronous background tasks* menggunakan Redis sebagai message broker, dilengkapi dengan Web UI monitoring dashboard ([Asynqmon](https://github.com/hibiken/asynqmon)) dan *health check endpoints*.

---

## 📌 Fitur Utama

- **Asynchronous Task Processing**: Pemrosesan antrean task secara asinkron dengan dukungan tingkatan prioritas multi-queue (`critical`, `default`, `low`).
- **HTTP Post Callback Dispatcher (`task:http_post`)**: Menjalankan tugas pemanggilan HTTP POST callback ke endpoint tujuan secara otomatis dengan header keamanan `X-Internal-Token`.
- **Integrated Web UI Monitoring (`Asynqmon`)**: Monitoring status antrean, active worker, task success/failed, serta retry history secara real-time melalui browser.
- **Health Check Endpoints**: Endpoint `/health` dan `/healthz` untuk kebutuhan Kubernetes liveness/readiness probe & Docker health check.
- **Graceful Shutdown**: Penanganan OS signals (`SIGINT`, `SIGTERM`) untuk menghentikan server worker dan dashboard HTTP dengan aman tanpa mengganggu task yang sedang diproses.
- **Structured JSON Logging**: Penggunaan `log/slog` dengan JSON handler untuk konsistensi log di lingkungan containerized / Cloud-Native.

---

## 🛠️ Arsitektur & Antrean Task

### Queue Priorities
Worker dikonfigurasi dengan **Concurrency: 5** dan prioritas bobot antrean sebagai berikut:

| Queue | Weight | Deskripsi |
| :--- | :--- | :--- |
| `critical` | 6 | Task prioritas tinggi (diproses lebih sering) |
| `default` | 3 | Task standar / umum |
| `low` | 1 | Task prioritas rendah (pemrosesan latar belakang) |

### Task Handlers

#### `task:http_post`
Mengirimkan request HTTP POST ke URL target dengan payload JSON sebagai berikut:

```json
{
  "url": "https://api.example.com/webhook",
  "body": "{\"event\": \"payment_success\", \"order_id\": \"12345\"}"
}
```

Setiap HTTP request yang dikirimkan oleh worker akan menyertakan header keamanan berikut:
```http
Content-Type: application/json
X-Internal-Token: <INTERNAL_TOKEN>
```

---

## 📋 Konfigurasi Environment Variables

Buat file `.env` di root project berdasarkan `.env.example`:

```env
REDIS_URL=localhost:6379
REDIS_USERNAME=
REDIS_PASSWORD=your_redis_password
INTERNAL_TOKEN=your_secure_internal_token
PORT=8085
```

### Detail Parameter

| Variable | Default | Deskripsi |
| :--- | :--- | :--- |
| `REDIS_URL` | `localhost:6379` | Host & port server Redis |
| `REDIS_USERNAME` | `""` | Username autentikasi Redis (opsional / Redis ACL) |
| `REDIS_PASSWORD` | `""` | Password autentikasi Redis (opsional) |
| `INTERNAL_TOKEN` | *default secret* | Token yang dikirim pada header `X-Internal-Token` saat HTTP callback |
| `PORT` | `8085` | Port HTTP untuk Asynqmon Dashboard & Health Check |

---

## 🚀 Cara Menjalankan

### 1. Menjalankan Secara Lokal (Go Native)

Pastikan Go 1.25+ dan Redis Server sudah terinstall dan berjalan.

```bash
# Move to directory
cd asynq-worker

# Copy environment configuration
cp .env.example .env

# Install dependencies
go mod download

# Run service
go run main.go
```

### 2. Menjalankan Menggunakan Docker

```bash
# Build Docker Image
docker build -t asynq-worker .

# Run Container
docker run -d \
  --name asynq-worker \
  -p 8085:8085 \
  --env-file .env \
  asynq-worker
```

---

## 📊 Monitoring & Health Checks

Saat service berjalan, Anda dapat mengakses:

- **Asynqmon Monitoring Dashboard**: [http://localhost:8085/](http://localhost:8085/)
- **Health Check Endpoint**: [http://localhost:8085/health](http://localhost:8085/health) atau `/healthz`

---

## 🚢 CI/CD & Deployment

Project ini menggunakan **GitHub Actions** (`.github/workflows/deploy.yml`) untuk deployment otomatis ke Kubernetes:

- **Image Registry**: `ghcr.io/exa-dev-coffe/worker-asynq`
- **App Name**: `worker-asynq`
- **Namespace**: `asynq`

Pipeline akan memicu build & rollout otomatis setiap terjadi push ke branch `main`.

---

## 📂 Struktur Project

```
.
├── config/
│   └── config.go      # Handling environment config via Viper
├── .github/
│   └── workflows/
│       └── deploy.yml # GitHub Actions CI/CD deployment pipeline
├── .env.example       # Template variabel lingkungan
├── Dockerfile         # Multi-stage Dockerfile (Go 1.25 Alpine)
├── go.mod / go.sum    # Manajamen dependensi Go module
├── main.go            # Entrypoint worker, handler task & HTTP server
└── README.md          # Dokumentasi teknis project
```
