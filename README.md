# 🏢 Indonesian Accounting System

A comprehensive, microservices-based accounting system designed specifically for Indonesian businesses, featuring multi-company support, Indonesian tax compliance (PPN), and modern web technologies.

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![React](https://img.shields.io/badge/react-18.2+-61DAFB?style=flat&logo=react)](https://reactjs.org/)
[![PostgreSQL](https://img.shields.io/badge/postgresql-15+-336791?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![Docker](https://img.shields.io/badge/docker-compose-2496ED?style=flat&logo=docker)](https://www.docker.com/)
[![License](https://img.shields.io/badge/license-Proprietary-red.svg)](LICENSE)

## 🌟 Key Features

### 📊 **Complete Accounting Suite**
- **Chart of Accounts** - Indonesian standard accounting structure
- **Journal Entries** - Double-entry bookkeeping with validation
- **General Ledger** - Real-time posting and balance tracking
- **Financial Reports** - Balance Sheet, P&L, Trial Balance, Cash Flow
- **Audit Trail** - Complete transaction history and change tracking

### 🇮🇩 **Indonesian Compliance**
- **PPN (VAT) Support** - Automatic 11% tax calculations
- **Indonesian Tax Rates** - PPh 21, PPh 23, PPh 4(2), PPh Badan
- **NPWP Validation** - Complete Tax ID verification with checksum
- **NIK Validation** - Indonesian ID number validation
- **Indonesian Currency** - IDR formatting without decimals
- **Jakarta Timezone** - Proper date/time handling (Asia/Jakarta)

### 🏢 **Business Management**
- **Multi-Company Support** - Manage multiple businesses from one system
- **Invoice Management** - Create, send, track invoices with email notifications
- **Vendor Management** - Supplier and purchase order management
- **Inventory Management** - Product tracking with stock movements
- **Customer Management** - Complete customer database with payment terms

### 🔐 **Security & Access Control**
- **Role-Based Access** - Admin, Manager, Accountant, User roles
- **JWT Authentication** - Secure token-based authentication
- **Indonesian Data Protection** - GDPR-compliant with local regulations
- **Rate Limiting** - API protection against abuse
- **CSRF Protection** - Cross-site request forgery prevention

### ⚡ **Modern Architecture**
- **Microservices** - 12 independent services
- **Go Workspace** - Modern Go development with shared modules
- **Progressive Web App** - Offline capabilities with service workers
- **Docker Support** - Containerized deployment
- **API Gateway** - Centralized routing and middleware
- **Health Monitoring** - Service health checks and metrics

## 🏗️ System Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   React PWA     │    │   API Gateway   │    │  Load Balancer  │
│   Frontend      │◄──►│    (Port 8000)  │◄──►│   (Optional)    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │
                ┌───────────────┼───────────────┐
                │               │               │
        ┌───────▼────┐  ┌───────▼────┐  ┌───────▼────┐
        │User Service│  │Account Svc │  │Transaction │
        │(Port 8001) │  │(Port 8002) │  │Service     │
        └────────────┘  └────────────┘  │(Port 8003) │
                                        └────────────┘
        ┌────────────┐  ┌────────────┐  ┌────────────┐
        │Invoice Svc │  │Vendor Svc  │  │Inventory   │
        │(Port 8004) │  │(Port 8005) │  │Service     │
        └────────────┘  └────────────┘  │(Port 8006) │
                                        └────────────┘
        ┌────────────┐  ┌────────────┐  ┌────────────┐
        │Report Svc  │  │Tax Service │  │Currency    │
        │(Port 8007) │  │(Port 8008) │  │Service     │
        └────────────┘  └────────────┘  │(Port 8009) │
                                        └────────────┘
        ┌────────────┐  ┌────────────┐
        │Notification│  │Company Svc │
        │(Port 8010) │  │(Port 8011) │
        └────────────┘  └────────────┘
                │
        ┌───────▼────────────────┐
        │    PostgreSQL 15       │
        │  (Multiple Databases)  │
        └────────────────────────┘
```

### 🎯 Service Overview

| Service | Port | Purpose | Database |
|---------|------|---------|----------|
| **API Gateway** | 8000 | Request routing, authentication, rate limiting | - |
| **User Service** | 8001 | Authentication, user management, companies | user_db |
| **Account Service** | 8002 | Chart of accounts, general ledger | account_db |
| **Transaction Service** | 8003 | Journal entries, posting, reversals | transaction_db |
| **Invoice Service** | 8004 | Billing, customer management | invoice_db |
| **Vendor Service** | 8005 | Supplier management, purchase orders | vendor_db |
| **Inventory Service** | 8006 | Product management, stock tracking | inventory_db |
| **Report Service** | 8007 | Financial reports, analytics | - |
| **Tax Service** | 8008 | Indonesian tax calculations | tax_db |
| **Currency Service** | 8009 | Exchange rates, currency conversion | - |
| **Notification Service** | 8010 | Email notifications, alerts | - |
| **Company Service** | 8011 | Multi-company management | user_db |

## 🚀 Quick Start

### Prerequisites

- **Go 1.21+** - [Download Go](https://golang.org/dl/)
- **Node.js 18+** - [Download Node.js](https://nodejs.org/)
- **PostgreSQL 15+** - [Download PostgreSQL](https://www.postgresql.org/download/)
- **Docker** (optional) - [Download Docker](https://www.docker.com/get-started)

### 1. Clone and Setup

```bash
# Clone the repository
git clone https://github.com/massehanto/accounting-system-go.git
cd accounting-system-go

# Make management script executable
chmod +x scripts/manage.sh

# Complete system setup (first-time installation)
./scripts/manage.sh setup
```

### 2. Configure Environment

```bash
# Copy and customize environment variables
cp .env.example .env

# Edit .env with your configuration
nano .env
```

**Required Environment Variables:**
```env
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_secure_password_here

# Security (CHANGE IN PRODUCTION)
JWT_SECRET=your-super-secure-jwt-secret-key-must-be-at-least-32-characters-long
SESSION_SECRET=your-session-secret-key-must-be-at-least-32-characters-long

# Indonesian Business Settings
DEFAULT_CURRENCY=IDR
DEFAULT_TIMEZONE=Asia/Jakarta
TAX_RATE_PPN=11.00
```

### 3. Start the System

```bash
# Start all services
./scripts/manage.sh start

# Check service status
./scripts/manage.sh status

# View service logs
./scripts/manage.sh logs api-gateway

# Start frontend (in separate terminal)
cd frontend && npm start
```

### 4. Access the Application

- **Frontend**: http://localhost:3000
- **API Gateway**: http://localhost:8000
- **API Documentation**: http://localhost:8000/api/docs
- **Health Check**: http://localhost:8000/health

**Default Login:**
- Email: `admin@contoh.co.id`
- Password: `password123`

## 🛠️ Development

### Go Workspace Development

This project uses Go workspaces for efficient development:

```bash
# Setup Go workspace
./scripts/manage.sh setup-workspace

# Sync workspace dependencies
./scripts/manage.sh sync-workspace

# Start service in watch mode (hot reload)
./scripts/manage.sh watch user-service
```

### Running Tests

```bash
# Run unit tests
./scripts/manage.sh test

# Run tests for specific service
./scripts/manage.sh test user-service unit

# Run integration tests
./scripts/manage.sh test all integration
```

### Database Management

```bash
# Setup databases
./scripts/manage.sh setup-db

# Run migrations
./scripts/manage.sh migrate

# Backup databases
./scripts/manage.sh backup
```

### Development Reset

```bash
# Reset development environment
./scripts/manage.sh dev-reset

# Clean temporary files
./scripts/manage.sh clean
```

## 🐳 Docker Deployment

### Quick Docker Start

```bash
# Build and start with Docker Compose
docker-compose up -d

# Check container status
docker-compose ps

# View logs
docker-compose logs -f api-gateway

# Stop services
docker-compose down
```

### Docker Management Scripts

```bash
# Build Docker images
./scripts/manage.sh docker-build

# Start with Docker Compose
./scripts/manage.sh docker-up

# Stop Docker services
./scripts/manage.sh docker-down
```

### Production Deployment

```bash
# Set production environment
export GO_ENV=production
export NODE_ENV=production

# Update environment variables for production
# - Use strong passwords (32+ characters)
# - Enable HTTPS
# - Configure proper database settings
# - Set up email SMTP

# Deploy with Docker Compose
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

## 📊 API Documentation

### Authentication

All API endpoints (except `/auth/login` and `/auth/register`) require JWT authentication:

```bash
# Login to get token
curl -X POST http://localhost:8000/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@contoh.co.id","password":"password123"}'

# Use token in subsequent requests
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8000/api/accounts
```

### Key Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/auth/login` | POST | User authentication |
| `/api/accounts` | GET/POST | Chart of accounts |
| `/api/transactions` | GET/POST | Journal entries |
| `/api/transactions/{id}/post` | POST | Post journal entry |
| `/api/invoices` | GET/POST | Invoice management |
| `/api/vendors` | GET/POST | Vendor management |
| `/api/reports/balance-sheet` | GET | Balance sheet report |
| `/api/calculate-tax` | POST | Tax calculations |

### Rate Limiting

- **Basic endpoints**: 60 requests/minute
- **Auth endpoints**: 20 requests/minute
- **Protected endpoints**: 100 requests/minute

## 🏢 Indonesian Business Features

### Tax Compliance

```json
// PPN (VAT) Calculation Example
{
  "amount": 1000000,
  "tax_rate_id": 1,
  "result": {
    "base_amount": 1000000,
    "tax_rate": 11.0,
    "tax_amount": 110000,
    "total": 1110000
  }
}
```

### Account Code Structure

Indonesian standard chart of accounts:

- **1000-1999**: Assets (Aset)
- **2000-2999**: Liabilities (Kewajiban)
- **3000-3999**: Equity (Ekuitas)
- **4000-4999**: Revenue (Pendapatan)
- **5000-5999**: Expenses (Biaya)

### Currency Handling

- All amounts in Indonesian Rupiah (IDR)
- No decimal places (whole numbers only)
- Proper Indonesian number formatting

## 🔍 Monitoring & Troubleshooting

### Health Checks

```bash
# Check all service health
curl http://localhost:8000/health

# Check specific service
curl http://localhost:8001/health

# View metrics
curl http://localhost:8000/metrics
```

### Common Issues

**Service won't start:**
```bash
# Check PostgreSQL is running
./scripts/manage.sh status

# View service logs
./scripts/manage.sh logs service-name

# Restart specific service
./scripts/manage.sh restart service-name
```

**Database connection issues:**
```bash
# Test database connection
pg_isready -h localhost -p 5432

# Reset database
./scripts/manage.sh setup-db
```

**Port conflicts:**
```bash
# Check what's using a port
lsof -i :8000

# Kill process on specific port
sudo kill -9 $(lsof -ti:8000)
```

### Log Locations

- **Service logs**: `logs/service-name.log`
- **System logs**: `logs/manage.log`
- **Error logs**: Check individual service logs

## 🔧 Configuration

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `DB_PASSWORD` | PostgreSQL password | - | ✅ |
| `JWT_SECRET` | JWT signing key (32+ chars) | - | ✅ |
| `SESSION_SECRET` | Session secret (32+ chars) | - | ✅ |
| `DEFAULT_CURRENCY` | Base currency | IDR | ❌ |
| `DEFAULT_TIMEZONE` | System timezone | Asia/Jakarta | ❌ |
| `TAX_RATE_PPN` | Indonesian VAT rate | 11.00 | ❌ |
| `SMTP_HOST` | Email server | - | ❌ |
| `SMTP_USER` | Email username | - | ❌ |
| `SMTP_PASSWORD` | Email password | - | ❌ |

### Security Configuration

**Production Security Checklist:**

- [ ] Use strong passwords (32+ characters)
- [ ] Enable HTTPS/TLS
- [ ] Configure proper CORS origins
- [ ] Set up firewall rules
- [ ] Enable database SSL
- [ ] Configure rate limiting
- [ ] Set up monitoring and alerts
- [ ] Regular security updates

## 🧪 Testing

### Unit Tests

```bash
# Run all unit tests
go test ./... -short

# Run tests with coverage
go test ./... -cover

# Run specific service tests
go test ./user-service/... -v
```

### Integration Tests

```bash
# Start test environment
./scripts/manage.sh start

# Run integration tests
go test ./... -tags=integration

# End-to-end tests
./scripts/manage.sh test all e2e
```

## 📈 Performance

### Optimization Tips

1. **Database Indexing** - All critical queries are indexed
2. **Connection Pooling** - Configured for optimal performance
3. **Caching** - Service worker caching for offline support
4. **Rate Limiting** - Prevents system overload
5. **Health Checks** - Automatic service recovery

### Monitoring

- Service health endpoints
- Performance metrics collection
- Response time tracking
- Error rate monitoring

## 🤝 Contributing

### Development Workflow

1. **Fork** the repository
2. **Create** a feature branch
3. **Make** your changes
4. **Test** thoroughly
5. **Submit** a pull request

### Code Standards

- **Go**: Follow Go conventions, use `gofmt`
- **React**: Use modern hooks, TypeScript preferred
- **Database**: Follow Indonesian accounting standards
- **API**: RESTful design, proper error handling
- **Security**: Follow OWASP guidelines

### Git Hooks

The project includes pre-commit hooks for:
- Go formatting (`gofmt`)
- Go vetting (`go vet`)
- Unit tests
- Linting

## 📚 Additional Resources

### Documentation

- [API Documentation](http://localhost:8000/api/docs) - OpenAPI/Swagger docs
- [Go Workspace Guide](https://go.dev/doc/tutorial/workspaces) - Go development
- [Indonesian Tax Laws](https://www.pajak.go.id/) - Tax compliance reference

### Indonesian Accounting Standards

- **SAK ETAP** - Financial accounting standards
- **PSAK** - Indonesian financial accounting standards
- **Tax Law** - Indonesian taxation regulations

### Support

- **Issues**: Use GitHub issues for bug reports
- **Discussions**: GitHub discussions for questions
- **Security**: Email security@domain.com for security issues

## 📄 License

This project is proprietary software. See [LICENSE](LICENSE) for details.

## 🙏 Acknowledgments

- Indonesian Institute of Accountants (IAI)
- Indonesian Tax Authority (Direktorat Jenderal Pajak)
- Go and React communities
- Contributors and maintainers

---

**🏢 Built for Indonesian Businesses | 🇮🇩 Compliant with Indonesian Regulations**

For questions or support, please open an issue or contact the development team.
