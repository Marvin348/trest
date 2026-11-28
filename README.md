# Trest – Deployment Infrastructure

A lightweight hosting platform designed to manage projects and applications, automate containerized deployments, and orchestrate workloads on Kubernetes through independent microservices.

### 🐳 Docker Setup

The application uses Docker Compose to run its services and supporting infrastructure.

- **PostgreSQL 17** – Database server
- **RabbitMQ** – Asynchronous messaging between services
- **Kubernetes** – Execution and management of application deployments

### 🛠️ Tech Stack

- **Language:** Go
- **Database:** PostgreSQL
- **Communication:** REST, gRPC, RabbitMQ
- **Architecture:** Microservices, Event Sourcing, Transactional Outbox
- **Infrastructure:** Docker, Docker Compose, Kubernetes
- **Testing:** Go Testing, testify, Testcontainers

### ▶️ Getting Started

**Prerequisites:**

- Docker and Docker Compose
- A running Kubernetes cluster
- A valid Kubernetes configuration (kubeconfig)

#### 1. Configure environment variables

Create the required .env files for Docker Compose and the individual services.

Configure the database credentials, RabbitMQ connection settings, and Kubernetes access.

#### 2. Start the application

From the directory containing the Docker Compose configuration:

```bash
docker compose up -d --build
```

#### 3. Verify running containers

```bash
docker compose ps
```

#### 4. View service logs

```bash
docker compose logs -f
```

#### 5. Stop the application

```bash
docker compose down
```

**Note:** Kubernetes must be running and accessible to the Deployment Worker for deployments to execute successfully.

