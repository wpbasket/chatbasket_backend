# Chatbasket Backend

<img src="https://img.shields.io/badge/Go-1.25.2-00ADD8?style=flat-square&logo=go" height="28" style="margin-left: 10px;border-radius: 10px; margin-bottom: 10px;">
<img src="https://img.shields.io/badge/Architecture-clean-success?style=flat-square" height="28" style="margin-left: 10px;border-radius: 10px; margin-bottom: 10px;">
<img src="https://img.shields.io/badge/Security-privacy--first-blueviolet?style=flat-square" height="28" style="margin-left: 10px;border-radius: 10px; margin-bottom: 10px;">


> **A simplified yet highly secure production-grade backend for the Chatbasket application.**
>
> **View Live Deployment:** [https://chatbasket.live](https://chatbasket.live)

## 🚀 Overview

Chatbasket Backend is the high-performance foundation for a **privacy-first social platform**, designed to solve the engineering challenge of balancing **Public Discovery** with **Private Security**.

It acts as a **strict privacy enforcement engine** that bridges the gap between open user profiling and encrypted, isolated personal networks. By leveraging Go's concurrency models, it delivers a seamless real-time experience without compromising the rigid security boundaries required for modern social interactions.

Unlike standard boilerplate implementations, this project is built for **real-world production**—featuring custom security protections, aggressive connection pooling strategies, and a "Zero-Touch" deployment pipeline.

---

## 🏗️ System Architecture

The system operates as a **Secure Gateway** (BFF - Backend for Frontend), ensuring that the frontend never interacts directly with sensitive database layers or auth providers.

**[Frontend Repository (Expo Web + Native)](https://github.com/wpbasket/chatbasket)**

```mermaid
graph TD
    User(["User Device"]) -->|HTTPS| CF["Cloudflare Edge<br>DDoS Protection"]
    CF -->|Strict SSL| Nginx["Nginx Reverse Proxy"]
    Nginx -->|Proxy| API["Go Backend API"]
    
    subgraph "Secure Zone"
    API -->|Validation| Layer1["Handlers"]
    Layer1 -->|Business Logic| Layer2["Services"]
    Layer2 -->|Persistence| Layer3["Repositories"]
    end

    Layer2 -.->|Server SDK| Appwrite["Appwrite (Auth/Storage)"]
    Layer3 -.->|TCP| DB[("PostgreSQL")]
```

#### 1. Clean Architecture & Dependency Injection
The codebase enforces a strict unidirectional dependency flow (`Handler` -> `Service` -> `Repository`). 
- **Decoupling**: Services and Repositories are explicitly injected via factory functions (e.g., `NewUserHandler(service)`), making the system highly modular.
- **Testability**: This pattern allows for effortless mocking of dependencies during unit testing.

#### 2. Secure Gateway Pattern
Crucially, **Appwrite** is used strictly as a backend infrastructure component via the Server API. The client (Expo) **never** holds API keys or talks to Appwrite directly. The Go API acts as the sole gatekeeper, enforcing business rules before any data persists.

#### 3. Code Design Principles
- **Strictly Typed Responses**: We avoid `map[string]interface{}`. All responses are defined in `model/` structs, ensuring the frontend has a predictable contract.
- **Centralized Error Handling**: A unified error model (`model.ApiError`) ensures that every failure returns a consistent JSON structure with actionable codes.

---

## 🔐 Security Architecture

Security is not an afterthought; it is baked into the core application flow.

- **Dual-Strategy Authentication**: The middleware (`middleware/session.go`) implements a flexible hybrid system:
    - **Native Apps**: Accepts standard `Authorization: Bearer <session_id>:<user_id>` headers.
    - **Web Clients**: Automatically detects and validates `HttpOnly` Secure Cookies, preventing XSS attacks.
    
- **Mandatory Two-Step Verification**: All sensitive entry points (Signup & Login) are enforced by a strict **2FA flow**. Users must verify ownership via OTP before receiving any session tokens.

- **Credential Hashing**: Sensitive One-Time Passwords (OTPs) are hashed using **Argon2id**, ensuring that even temporary credentials are stored securely.

---

## ☁️ Infrastructure & Reliability

The application operates on a hardened cloud infrastructure designed for zero-trust security and high availability.

#### 1. High-Performance Engineering
- **Advanced Connection Pooling**: The PostgreSQL pool is manually tuned with `MaxConnLifetimeJitter` and strict resource caps to prevent thundering herd issues.
- **Graceful Shutdown**: The server captures OS signals (`SIGTERM`) to finish in-flight requests and close DB connections cleanly, ensuring zero dropped requests during deployments.
- **Hybrid Rate Limiting**: Features **Cloudflare WAF** for edge-level DDoS mitigation, backed by an in-memory application limiter for granular endpoint protection.

#### 2. Zero Trust Network
- **DigitalOcean Droplet**: Hosted on scalable compute instances for reliable performance.
- **Zero Trust Security Architecture**:
    1. **Strict SSL (Identity)**: Encrypted via a 15-year Cloudflare Origin Certificate to prevent Man-in-the-Middle attacks.
    2. **Mutual TLS (Access)**: Enforced **Authenticated Origin Pulls**, requiring Nginx to validate Cloudflare's cryptographic signature for every request.
    3. **IP Firewall (Perimeter)**: Nginx strictly whitelists official Cloudflare CIDR ranges and drops all direct traffic.
- **Reverse Proxy**: Nginx handles SSL termination and header sanitization before requests reach the Go application.

---

## 🛠️ Tech Stack

We choose tools that offer **Control** and **Predictability**.

| Component | Technology | Rationale (Why?) |
|-----------|------------|------------------|
| **Core Logic** | ![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=flat-square&logo=go&logoColor=white) | **Concurrency & Safety**: Goroutines handle thousands of concurrent WebSocket connections with minimal footprint. |
| **API Framework** | ![Echo](https://img.shields.io/badge/Echo_v4-00ADD8?style=flat-square&logoColor=white) | **Performance**: Zero-allocation router that is significantly faster than Gin or Fiber in our benchmarks. |
| **Database** | ![Postgres](https://img.shields.io/badge/postgres-%23316192.svg?style=flat-square&logo=postgresql&logoColor=white) | **Data Integrity**: Strict ACID compliance for user transactions and relations. |
| **Authentication** | ![Appwrite](https://img.shields.io/badge/Appwrite-%23FD366E.svg?style=flat-square&logo=appwrite&logoColor=white) | **Managed Security**: Offloads session token management while sticking to a self-hostable open-source standard. |
| **Object Storage** | ![Appwrite](https://img.shields.io/badge/Appwrite_Storage-%23FD366E.svg?style=flat-square&logo=appwrite&logoColor=white) | **Secure Uploads**: Handles chunked uploads and virus scanning for media. |
| **Edge Security** | ![Cloudflare](https://img.shields.io/badge/Cloudflare-F38020?style=flat-square&logo=Cloudflare&logoColor=white) | **Zero Trust Gateway**: Beyond simple DDoS protection, it acts as an mTLS firewall (Authenticated Origin Pulls) and enforces strict end-to-end encryption, completely isolating the origin infrastructure. |
| **CI/CD** | ![GitHub Actions](https://img.shields.io/badge/github%20actions-%232671E5.svg?style=flat-square&logo=githubactions&logoColor=white) | **Zero-Touch Deploy**: Commits to `main` automatically build and swap containers on DigitalOcean. |

---

## 🔮 Future Roadmap

We are actively developing advanced intelligence and architecture upgrades:

- **🔐 Native Secure Authentication (In Development)**:
    - **Custom Auth Service**: Migrating away from generic providers.
    - **Hybrid Session Strategy**: Implementing **JWT + Database Persistence**. This enables real-time tracking of active devices and allows users to **terminate specific sessions**, a critical security feature often missing in standard implementations.

- **🔐 Advanced Privacy Layer (In Development)**:
    - **Zero-Knowledge Privacy**: Embedding ChaCha20-Poly1305 encryption for sensitive user fields.
    - **Blind Indexing**: Implementing HMAC-SHA256 for secure, opaque user lookups.

- **📱 Cross-Platform Notifications (Upcoming)**:
    - **FCM Universal Integration**: Unified push delivery for Android & iOS.
    - **Dual-Payload Strategy**: Support for both System Alerts (`Notification`) and Silent Data Updates (`Data-Only`).

- **🤖 AI & Vector Engine (Upcoming)**:
    - **Azure Cosmos DB**: Serving as a high-dimensional **Vector Store** for RAG pipelines.
    - **Semantic Search**: Enabling natural language discovery of profiles and content.

---

## 💻 Running the Project

#### Local Development (Standard)
The `main` package is located in `app/`, keeping the root clean.

```bash
# Clone and tidy
git clone https://github.com/wpbasket/chatbasket-backend
cd chatbasket-api
go mod tidy

# Run the server
go run ./app
```

#### Production Build (Docker)
You can test the container build locally:

```bash
# Build optimized image
docker build -t chatbasket-api .

# Run with local environment variables
docker run -p 8080:8080 --env-file .env chatbasket-api
```
