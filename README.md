# Trest – Deployment Infrastructure

A lightweight hosting platform built with **Go**, **PostgreSQL**, and **Docker**, designed to manage projects, deploy containerized applications, and orchestrate deployments through independent microservices.

### 🐳 Docker Setup

The infrastructure runs with **Docker Compose** and currently includes:

- **PostgreSQL 17** – Database server
- **projects_db** – Database for the Project Service
- **deployments_db** – Database for the Deployment Service
- **Separate database users** for service isolation
- **Healthchecks** to monitor database availability

### 🛠️ Tech Stack

- Go
- PostgreSQL
- Docker & Docker Compose
- REST APIs
- gRPC

### ▶️ Getting Started

Create a `.env` file with the required database credentials.

Start the infrastructure:

```bash
docker compose up -d
```

Check running containers:

```bash
docker compose ps
```

Stop the infrastructure:

```bash
docker compose down
```

**Status:** 🚧 Work in progress – more coming soon!
