#!/bin/bash
# scripts/manage.sh - Enhanced version with Go workspace support and additional features

set -e

# Enhanced color definitions
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly PURPLE='\033[0;35m'
readonly CYAN='\033[0;36m'
readonly WHITE='\033[1;37m'
readonly NC='\033[0m'

# Configuration
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
readonly LOG_DIR="$PROJECT_ROOT/logs"
readonly PID_DIR="$PROJECT_ROOT/pids"
readonly BACKUP_DIR="$PROJECT_ROOT/backups"
readonly WORKSPACE_FILE="$PROJECT_ROOT/go.work"

# Service definitions with enhanced metadata
declare -A SERVICES=(
    ["user-service"]="8001"
    ["account-service"]="8002"
    ["transaction-service"]="8003"
    ["invoice-service"]="8004"
    ["vendor-service"]="8005"
    ["inventory-service"]="8006"
    ["report-service"]="8007"
    ["tax-service"]="8008"
    ["currency-service"]="8009"
    ["notification-service"]="8010"
    ["api-gateway"]="8000"
    ["company-service"]="8011"
)

declare -A SERVICE_DEPENDENCIES=(
    ["api-gateway"]="user-service account-service"
    ["transaction-service"]="account-service"
    ["report-service"]="account-service transaction-service"
    ["invoice-service"]="user-service"
    ["vendor-service"]="user-service"
    ["inventory-service"]="user-service"
)

readonly DATABASES=("user_db" "account_db" "transaction_db" "invoice_db" "vendor_db" "inventory_db" "tax_db")
readonly REQUIRED_GO_VERSION="1.19"
readonly REQUIRED_NODE_VERSION="16.0"

# Performance monitoring
declare -A SERVICE_METRICS
readonly METRICS_FILE="$LOG_DIR/metrics.json"

# Logging functions with enhanced formatting
log_info() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')] [INFO]${NC} $1" | tee -a "$LOG_DIR/manage.log" 2>/dev/null || echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] [SUCCESS]${NC} $1" | tee -a "$LOG_DIR/manage.log" 2>/dev/null || echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] [WARNING]${NC} $1" | tee -a "$LOG_DIR/manage.log" 2>/dev/null || echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] [ERROR]${NC} $1" >&2
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] [ERROR] $1" >> "$LOG_DIR/manage.log" 2>/dev/null || true
}

log_debug() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        echo -e "${PURPLE}[$(date +'%Y-%m-%d %H:%M:%S')] [DEBUG]${NC} $1" | tee -a "$LOG_DIR/manage.log" 2>/dev/null || echo -e "${PURPLE}[DEBUG]${NC} $1"
    fi
}

# Enhanced utility functions
setup_directories() {
    local dirs=("$LOG_DIR" "$PID_DIR" "$BACKUP_DIR" "$PROJECT_ROOT/uploads" "$PROJECT_ROOT/tmp")
    for dir in "${dirs[@]}"; do
        mkdir -p "$dir"
    done
    
    # Set appropriate permissions
    chmod 755 "$LOG_DIR" "$PID_DIR" "$BACKUP_DIR"
    find "$SCRIPT_DIR" -name "*.sh" -exec chmod +x {} \; 2>/dev/null || true
    
    # Create metrics file if it doesn't exist
    [[ ! -f "$METRICS_FILE" ]] && echo '{}' > "$METRICS_FILE"
}

check_command() {
    if ! command -v "$1" &> /dev/null; then
        log_error "$1 is not installed or not in PATH"
        return 1
    fi
}

check_version() {
    local cmd=$1
    local required_version=$2
    local current_version
    
    case $cmd in
        "go")
            current_version=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
            ;;
        "node")
            current_version=$(node --version | sed 's/v//')
            ;;
        *)
            return 1
            ;;
    esac
    
    if version_compare "$current_version" "$required_version"; then
        return 0
    else
        log_warning "$cmd version $current_version is below recommended $required_version"
        return 1
    fi
}

version_compare() {
    printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

wait_for_service() {
    local service=$1
    local port=$2
    local timeout=${3:-30}
    local count=0
    local start_time=$(date +%s)

    log_info "Waiting for $service to be ready on port $port..."
    
    while [ $count -lt $timeout ]; do
        if curl -s -f "http://localhost:$port/health" > /dev/null 2>&1; then
            local end_time=$(date +%s)
            local startup_time=$((end_time - start_time))
            log_success "$service is ready (startup time: ${startup_time}s)"
            record_metric "$service" "startup_time" "$startup_time"
            return 0
        fi
        
        sleep 1
        ((count++))
        
        if [ $((count % 5)) -eq 0 ]; then
            log_info "Still waiting for $service... (${count}s/${timeout}s)"
        fi
    done
    
    log_error "$service failed to become ready within $timeout seconds"
    return 1
}

# Go workspace management
setup_workspace() {
    log_info "Setting up Go workspace..."
    
    cd "$PROJECT_ROOT"
    
    if [[ ! -f "$WORKSPACE_FILE" ]]; then
        log_info "Creating go.work file..."
        go work init
        
        # Add shared module first
        if [[ -d "shared" ]]; then
            go work use ./shared
            log_debug "Added shared module to workspace"
        fi
        
        # Add all services
        for service in "${!SERVICES[@]}"; do
            if [[ -d "$service" && -f "$service/go.mod" ]]; then
                go work use "./$service"
                log_debug "Added $service to workspace"
            fi
        done
        
        log_success "Go workspace created with all modules"
    else
        log_info "Go workspace already exists"
    fi
    
    # Sync workspace
    sync_workspace
}

sync_workspace() {
    log_info "Syncing Go workspace..."
    cd "$PROJECT_ROOT"
    
    if [[ -f "$WORKSPACE_FILE" ]]; then
        go work sync
        log_success "Workspace synchronized"
    else
        log_warning "No go.work file found. Run 'setup' command first."
        return 1
    fi
}

validate_workspace() {
    log_info "Validating Go workspace..."
    cd "$PROJECT_ROOT"
    
    if [[ ! -f "$WORKSPACE_FILE" ]]; then
        log_error "No go.work file found"
        return 1
    fi
    
    # Check if all services are in workspace
    local missing_services=()
    for service in "${!SERVICES[@]}"; do
        if [[ -d "$service" ]] && ! grep -q "./$service" "$WORKSPACE_FILE"; then
            missing_services+=("$service")
        fi
    done
    
    if [[ ${#missing_services[@]} -gt 0 ]]; then
        log_warning "Services not in workspace: ${missing_services[*]}"
        return 1
    fi
    
    log_success "Workspace validation passed"
    return 0
}

# Enhanced requirement checking
check_requirements() {
    log_info "Checking system requirements..."
    
    local missing_deps=()
    local version_warnings=()
    local required_commands=("go" "node" "npm" "psql" "curl" "wget" "docker" "git")
    
    for cmd in "${required_commands[@]}"; do
        if ! check_command "$cmd"; then
            if [[ "$cmd" == "docker" ]]; then
                log_warning "Docker not found - container operations will be unavailable"
            else
                missing_deps+=("$cmd")
            fi
        fi
    done
    
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_info "Please install the missing dependencies and try again"
        return 1
    fi
    
    # Check versions
    check_version "go" "$REQUIRED_GO_VERSION" || version_warnings+=("Go")
    check_version "node" "$REQUIRED_NODE_VERSION" || version_warnings+=("Node.js")
    
    if [[ ${#version_warnings[@]} -gt 0 ]]; then
        log_warning "Version warnings for: ${version_warnings[*]}"
    fi
    
    # Check system resources
    check_system_resources
    
    log_success "System requirements check completed"
    return 0
}

check_system_resources() {
    local available_memory_gb
    local available_disk_gb
    
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        available_memory_gb=$(( $(sysctl -n hw.memsize) / 1024 / 1024 / 1024 ))
        available_disk_gb=$(df -g . | awk 'NR==2 {print $4}')
    else
        # Linux
        available_memory_gb=$(( $(grep MemAvailable /proc/meminfo | awk '{print $2}') / 1024 / 1024 ))
        available_disk_gb=$(df -BG . | awk 'NR==2 {print $4}' | sed 's/G//')
    fi
    
    log_debug "Available memory: ${available_memory_gb}GB"
    log_debug "Available disk space: ${available_disk_gb}GB"
    
    if [[ $available_memory_gb -lt 4 ]]; then
        log_warning "Low memory: ${available_memory_gb}GB available (recommended: 4GB+)"
    fi
    
    if [[ $available_disk_gb -lt 10 ]]; then
        log_warning "Low disk space: ${available_disk_gb}GB available (recommended: 10GB+)"
    fi
}

# Enhanced environment setup
setup_environment() {
    log_info "Setting up development environment..."
    
    setup_directories
    setup_workspace
    
    # Copy environment file if it doesn't exist
    if [[ ! -f "$PROJECT_ROOT/.env" ]]; then
        if [[ -f "$PROJECT_ROOT/.env.example" ]]; then
            cp "$PROJECT_ROOT/.env.example" "$PROJECT_ROOT/.env"
            log_success "Created .env from .env.example"
            log_warning "Please update .env file with your configuration before proceeding"
        else
            log_error ".env.example not found. Creating minimal .env file"
            create_minimal_env_file
        fi
    else
        log_info ".env file already exists"
    fi
    
    # Validate environment
    if ! validate_environment; then
        log_error "Environment validation failed"
        return 1
    fi
    
    # Set up Git hooks if in Git repository
    setup_git_hooks
    
    log_success "Environment setup complete"
    return 0
}

setup_git_hooks() {
    if [[ -d "$PROJECT_ROOT/.git" ]]; then
        log_info "Setting up Git hooks..."
        
        local hooks_dir="$PROJECT_ROOT/.git/hooks"
        
        # Pre-commit hook for Go formatting and linting
        cat > "$hooks_dir/pre-commit" << 'EOF'
#!/bin/bash
echo "Running pre-commit checks..."

# Format Go code
go fmt ./...

# Run Go vet
if ! go vet ./...; then
    echo "go vet failed"
    exit 1
fi

# Run tests
if ! go test ./... -short; then
    echo "Tests failed"
    exit 1
fi

echo "Pre-commit checks passed"
EOF
        
        chmod +x "$hooks_dir/pre-commit"
        log_success "Git hooks installed"
    fi
}

create_minimal_env_file() {
    cat > "$PROJECT_ROOT/.env" << 'EOF'
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_secure_password_here

# JWT Configuration - CHANGE THESE IN PRODUCTION
JWT_SECRET=your-super-secure-jwt-secret-key-must-be-at-least-32-characters-long
JWT_EXPIRATION=86400

# Session Security
SESSION_SECRET=your-session-secret-key-must-be-at-least-32-characters-long
BCRYPT_COST=12

# Indonesian Business Configuration
DEFAULT_CURRENCY=IDR
DEFAULT_TIMEZONE=Asia/Jakarta
TAX_RATE_PPN=11.00

# Service Ports (default values)
API_GATEWAY_PORT=8000
USER_SERVICE_PORT=8001
ACCOUNT_SERVICE_PORT=8002
TRANSACTION_SERVICE_PORT=8003
INVOICE_SERVICE_PORT=8004
VENDOR_SERVICE_PORT=8005
INVENTORY_SERVICE_PORT=8006
REPORT_SERVICE_PORT=8007
TAX_SERVICE_PORT=8008
CURRENCY_SERVICE_PORT=8009
NOTIFICATION_SERVICE_PORT=8010
COMPANY_SERVICE_PORT=8011

# Frontend Configuration
REACT_APP_API_URL=http://localhost:8000/api
FRONTEND_URL=http://localhost:3000

# Development Environment
NODE_ENV=development
GO_ENV=development

# Debug Mode
DEBUG=false
EOF

    log_warning "Created minimal .env file. Please update with your actual configuration!"
}

validate_environment() {
    log_info "Validating environment configuration..."
    
    # Check if .env file exists and is readable
    if [[ ! -r "$PROJECT_ROOT/.env" ]]; then
        log_error ".env file not found or not readable"
        return 1
    fi
    
    # Source the .env file
    set -a
    source "$PROJECT_ROOT/.env" 2>/dev/null || {
        log_error "Failed to load .env file"
        return 1
    }
    set +a
    
    # Check critical variables
    local critical_vars=("DB_PASSWORD" "JWT_SECRET" "SESSION_SECRET")
    local missing_vars=()
    
    for var in "${critical_vars[@]}"; do
        if [[ -z "${!var}" ]]; then
            missing_vars+=("$var")
        fi
    done
    
    if [[ ${#missing_vars[@]} -gt 0 ]]; then
        log_error "Missing critical environment variables: ${missing_vars[*]}"
        return 1
    fi
    
    # Check JWT_SECRET length
    if [[ ${#JWT_SECRET} -lt 32 ]]; then
        log_error "JWT_SECRET must be at least 32 characters long"
        return 1
    fi
    
    # Validate Indonesian business settings
    if [[ "$DEFAULT_CURRENCY" != "IDR" ]]; then
        log_warning "DEFAULT_CURRENCY is not IDR - may not comply with Indonesian regulations"
    fi
    
    if [[ "$DEFAULT_TIMEZONE" != "Asia/Jakarta" ]]; then
        log_warning "DEFAULT_TIMEZONE is not Asia/Jakarta - may cause issues with Indonesian business hours"
    fi
    
    log_success "Environment validation passed"
    return 0
}

# Enhanced dependency installation
install_dependencies() {
    log_info "Installing project dependencies..."
    
    cd "$PROJECT_ROOT"
    
    # Ensure workspace is set up
    if [[ ! -f "$WORKSPACE_FILE" ]]; then
        setup_workspace
    fi
    
    # Install Go dependencies using workspace
    log_info "Installing Go dependencies via workspace..."
    if go work sync; then
        log_success "Go workspace dependencies synchronized"
    else
        log_error "Failed to sync Go workspace"
        return 1
    fi
    
    # Download all dependencies
    log_info "Downloading Go modules..."
    go mod download all
    
    # Install frontend dependencies
    if [[ -d "frontend" && -f "frontend/package.json" ]]; then
        log_info "Installing frontend dependencies..."
        (cd "frontend" && npm ci --production=false) || {
            log_error "Failed to install frontend dependencies"
            return 1
        }
    fi
    
    # Install development tools
    install_dev_tools
    
    log_success "All dependencies installed successfully"
    return 0
}

install_dev_tools() {
    log_info "Installing development tools..."
    
    local tools=(
        "github.com/air-verse/air@latest"
        "golang.org/x/tools/cmd/goimports@latest"
        "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
        "github.com/pressly/goose/v3/cmd/goose@latest"
    )
    
    for tool in "${tools[@]}"; do
        log_debug "Installing $tool"
        go install "$tool" || log_warning "Failed to install $tool"
    done
    
    log_success "Development tools installed"
}

# Enhanced database operations
check_postgresql_status() {
    if pgrep -x "postgres" > /dev/null || pgrep -x "postgresql" > /dev/null; then
        log_debug "PostgreSQL is running"
        return 0
    else
        log_debug "PostgreSQL is not running"
        return 1
    fi
}

start_postgresql() {
    log_info "Starting PostgreSQL..."
    
    if command -v systemctl &> /dev/null; then
        sudo systemctl start postgresql
    elif command -v brew &> /dev/null; then
        brew services start postgresql
    elif command -v pg_ctl &> /dev/null; then
        pg_ctl -D /usr/local/var/postgres start
    elif command -v docker &> /dev/null; then
        # Try Docker as fallback
        start_postgresql_docker
        return $?
    else
        log_warning "Cannot automatically start PostgreSQL. Please start it manually."
        return 1
    fi
    
    # Wait for PostgreSQL to start
    local count=0
    while [[ $count -lt 30 ]]; do
        if check_postgresql_status && pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
            log_success "PostgreSQL started successfully"
            return 0
        fi
        sleep 1
        ((count++))
    done
    
    log_error "PostgreSQL failed to start"
    return 1
}

start_postgresql_docker() {
    log_info "Starting PostgreSQL using Docker..."
    
    local container_name="accounting-postgres"
    
    # Check if container already exists
    if docker ps -a --format 'table {{.Names}}' | grep -q "$container_name"; then
        log_info "Starting existing PostgreSQL container..."
        docker start "$container_name"
    else
        log_info "Creating new PostgreSQL container..."
        docker run -d \
            --name "$container_name" \
            -e POSTGRES_PASSWORD="${DB_PASSWORD:-postgres}" \
            -e POSTGRES_USER="${DB_USER:-postgres}" \
            -p 5432:5432 \
            postgres:15-alpine
    fi
    
    # Wait for container to be ready
    local count=0
    while [[ $count -lt 30 ]]; do
        if docker exec "$container_name" pg_isready -U "${DB_USER:-postgres}" >/dev/null 2>&1; then
            log_success "PostgreSQL Docker container is ready"
            return 0
        fi
        sleep 1
        ((count++))
    done
    
    log_error "PostgreSQL Docker container failed to start"
    return 1
}

setup_database() {
    log_info "Setting up PostgreSQL databases..."
    
    if ! check_postgresql_status; then
        if ! start_postgresql; then
            log_error "Cannot proceed without PostgreSQL"
            return 1
        fi
    fi
    
    # Wait a moment for PostgreSQL to be fully ready
    sleep 2
    
    # Check if we can connect to PostgreSQL
    if ! pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
        log_error "Cannot connect to PostgreSQL"
        return 1
    fi
    
    # Create databases using init script
    if [[ -f "$PROJECT_ROOT/database/init-db.sql" ]]; then
        log_info "Executing database initialization script..."
        
        # Try different connection methods
        if docker ps --format 'table {{.Names}}' | grep -q "accounting-postgres"; then
            # Docker container
            docker exec -i accounting-postgres psql -U "${DB_USER:-postgres}" < "$PROJECT_ROOT/database/init-db.sql"
        elif sudo -u postgres psql -c '\q' 2>/dev/null; then
            # System postgres user
            sudo -u postgres psql -f "$PROJECT_ROOT/database/init-db.sql"
        elif psql -h localhost -p 5432 -U "${DB_USER:-postgres}" -c '\q' 2>/dev/null; then
            # Direct connection
            PGPASSWORD="${DB_PASSWORD}" psql -h localhost -p 5432 -U "${DB_USER:-postgres}" -f "$PROJECT_ROOT/database/init-db.sql"
        else
            log_error "Cannot connect to PostgreSQL with any method"
            return 1
        fi
        
        log_success "Database initialization completed"
    else
        log_error "Database initialization script not found at database/init-db.sql"
        return 1
    fi
    
    return 0
}

# Database migration support
run_migrations() {
    local service=${1:-"all"}
    
    if ! command -v goose &> /dev/null; then
        log_error "goose migration tool not found. Install with: go install github.com/pressly/goose/v3/cmd/goose@latest"
        return 1
    fi
    
    log_info "Running database migrations for: $service"
    
    if [[ "$service" == "all" ]]; then
        for svc in "${!SERVICES[@]}"; do
            run_service_migrations "$svc"
        done
    else
        run_service_migrations "$service"
    fi
}

run_service_migrations() {
    local service=$1
    local migrations_dir="$PROJECT_ROOT/$service/migrations"
    
    if [[ -d "$migrations_dir" ]]; then
        log_info "Running migrations for $service..."
        cd "$migrations_dir"
        
        local db_name="${service/-/_}_db"
        goose postgres "host=localhost port=5432 user=${DB_USER} password=${DB_PASSWORD} dbname=${db_name} sslmode=disable" up
        
        log_success "Migrations completed for $service"
    else
        log_debug "No migrations found for $service"
    fi
}

# Enhanced service management with dependency handling
start_service() {
    local service=$1
    local port=${SERVICES[$service]}
    
    if [[ -z "$port" ]]; then
        log_error "Unknown service: $service"
        return 1
    fi
    
    if [[ ! -d "$PROJECT_ROOT/$service" ]]; then
        log_error "Service directory not found: $service"
        return 1
    fi
    
    # Check dependencies first
    if ! check_service_dependencies "$service"; then
        log_error "Service dependencies not met for $service"
        return 1
    fi
    
    # Check if service is already running
    if [[ -f "$PID_DIR/$service.pid" ]]; then
        local pid=$(cat "$PID_DIR/$service.pid")
        if kill -0 "$pid" 2>/dev/null; then
            log_warning "$service is already running (PID: $pid)"
            return 0
        else
            rm -f "$PID_DIR/$service.pid"
        fi
    fi
    
    log_info "Starting $service on port $port..."
    
    cd "$PROJECT_ROOT/$service"
    
    # Use air for hot reloading in development if available
    local start_cmd="go run ."
    if command -v air &> /dev/null && [[ -f ".air.toml" ]] && [[ "${GO_ENV:-development}" == "development" ]]; then
        start_cmd="air"
        log_debug "Using air for hot reloading"
    fi
    
    nohup $start_cmd > "$LOG_DIR/$service.log" 2>&1 &
    local pid=$!
    echo $pid > "$PID_DIR/$service.pid"
    
    # Wait for service to start
    if wait_for_service "$service" "$port" 30; then
        log_success "$service started successfully (PID: $pid)"
        record_service_start "$service" "$pid"
        return 0
    else
        log_error "Failed to start $service"
        # Cleanup PID file if service failed to start
        rm -f "$PID_DIR/$service.pid"
        return 1
    fi
}

check_service_dependencies() {
    local service=$1
    local deps="${SERVICE_DEPENDENCIES[$service]:-}"
    
    if [[ -z "$deps" ]]; then
        return 0
    fi
    
    log_debug "Checking dependencies for $service: $deps"
    
    for dep in $deps; do
        if ! is_service_running "$dep"; then
            log_warning "Dependency $dep is not running for $service"
            return 1
        fi
    done
    
    return 0
}

is_service_running() {
    local service=$1
    local port=${SERVICES[$service]}
    
    [[ -f "$PID_DIR/$service.pid" ]] && \
    kill -0 "$(cat "$PID_DIR/$service.pid")" 2>/dev/null && \
    curl -s -f "http://localhost:$port/health" >/dev/null 2>&1
}

# Enhanced service stopping with graceful shutdown
stop_service() {
    local service=$1
    local force=${2:-false}
    
    if [[ ! -f "$PID_DIR/$service.pid" ]]; then
        log_warning "$service is not running (no PID file found)"
        return 0
    fi
    
    local pid=$(cat "$PID_DIR/$service.pid")
    
    if ! kill -0 "$pid" 2>/dev/null; then
        log_warning "$service is not running (PID $pid not found)"
        rm -f "$PID_DIR/$service.pid"
        return 0
    fi
    
    log_info "Stopping $service (PID: $pid)..."
    
    # Try graceful shutdown first
    if kill -TERM "$pid" 2>/dev/null; then
        # Wait for graceful shutdown
        local count=0
        local timeout=15
        
        if [[ "$force" == "true" ]]; then
            timeout=5
        fi
        
        while [[ $count -lt $timeout ]]; do
            if ! kill -0 "$pid" 2>/dev/null; then
                rm -f "$PID_DIR/$service.pid"
                log_success "$service stopped gracefully"
                record_service_stop "$service"
                return 0
            fi
            sleep 1
            ((count++))
        done
        
        # Force kill if graceful shutdown failed
        log_warning "$service did not stop gracefully, forcing shutdown..."
        if kill -KILL "$pid" 2>/dev/null; then
            rm -f "$PID_DIR/$service.pid"
            log_success "$service force-stopped"
            record_service_stop "$service"
            return 0
        fi
    fi
    
    log_error "Failed to stop $service"
    return 1
}

start_all_services() {
    log_info "Starting Indonesian Accounting System services..."
    
    if ! check_postgresql_status; then
        if ! start_postgresql; then
            log_error "Cannot start services without PostgreSQL"
            return 1
        fi
    fi
    
    setup_directories
    
    # Start services in dependency order
    local service_order=("user-service" "company-service" "account-service" "transaction-service" 
                         "invoice-service" "vendor-service" "inventory-service" "tax-service" 
                         "currency-service" "notification-service" "report-service" "api-gateway")
    
    local failed_services=()
    local start_time=$(date +%s)
    
    for service in "${service_order[@]}"; do
        if [[ -d "$PROJECT_ROOT/$service" ]]; then
            if ! start_service "$service"; then
                failed_services+=("$service")
            fi
            # Small delay between service starts to avoid resource contention
            sleep 2
        else
            log_debug "Service directory not found: $service (skipping)"
        fi
    done
    
    local end_time=$(date +%s)
    local total_time=$((end_time - start_time))
    
    if [[ ${#failed_services[@]} -gt 0 ]]; then
        log_error "Failed to start services: ${failed_services[*]}"
        log_info "Check individual service logs in $LOG_DIR/"
        return 1
    fi
    
    log_success "All services started successfully in ${total_time}s!"
    print_service_info
    
    return 0
}

print_service_info() {
    echo
    log_info "🌐 Service URLs:"
    echo -e "  ${CYAN}API Gateway:${NC}     http://localhost:8000"
    echo -e "  ${CYAN}Health Check:${NC}    http://localhost:8000/health"
    echo -e "  ${CYAN}Frontend:${NC}        http://localhost:3000 (run 'npm start' in frontend directory)"
    echo
    log_info "📋 Monitoring:"
    echo -e "  ${CYAN}Service logs:${NC}    $LOG_DIR/"
    echo -e "  ${CYAN}Process IDs:${NC}     $PID_DIR/"
    echo -e "  ${CYAN}Metrics:${NC}         $METRICS_FILE"
    echo
    log_info "🔧 Management:"
    echo -e "  ${CYAN}Service status:${NC}  ./scripts/manage.sh status"
    echo -e "  ${CYAN}View logs:${NC}       ./scripts/manage.sh logs [service]"
    echo -e "  ${CYAN}Stop services:${NC}   ./scripts/manage.sh stop"
    echo
}

stop_all_services() {
    log_info "Stopping all services..."
    
    local stopped_count=0
    local failed_count=0
    
    # Stop services in reverse dependency order
    local service_order=("api-gateway" "report-service" "notification-service" "currency-service" 
                         "tax-service" "inventory-service" "vendor-service" "invoice-service" 
                         "transaction-service" "account-service" "company-service" "user-service")
    
    for service in "${service_order[@]}"; do
        if stop_service "$service"; then
            ((stopped_count++))
        else
            ((failed_count++))
        fi
    done
    
    # Clean up any remaining processes
    cleanup_stray_processes
    
    log_success "Stopped $stopped_count services"
    if [[ $failed_count -gt 0 ]]; then
        log_warning "$failed_count services had stop issues"
    fi
    
    return 0
}

cleanup_stray_processes() {
    # Clean up any remaining Go processes
    pkill -f "go run" 2>/dev/null || true
    pkill -f "air" 2>/dev/null || true
    
    # Clean up any stray service processes by port
    for port in "${SERVICES[@]}"; do
        local pid=$(lsof -ti:$port 2>/dev/null || true)
        if [[ -n "$pid" ]]; then
            log_debug "Killing process on port $port (PID: $pid)"
            kill -TERM "$pid" 2>/dev/null || true
        fi
    done
}

restart_service() {
    local service=$1
    
    if [[ -z "$service" ]]; then
        restart_all_services
        return $?
    fi
    
    log_info "Restarting $service..."
    stop_service "$service"
    sleep 2
    start_service "$service"
}

restart_all_services() {
    log_info "Restarting Indonesian Accounting System..."
    
    stop_all_services
    sleep 3
    start_all_services
}

# Enhanced status checking with health metrics
check_service_status() {
    local service=$1
    local port=${SERVICES[$service]}
    local status_code=1
    
    if [[ ! -f "$PID_DIR/$service.pid" ]]; then
        echo -e "  ${RED}●${NC} $service - Not running (no PID file)"
        return 1
    fi
    
    local pid=$(cat "$PID_DIR/$service.pid")
    
    if ! kill -0 "$pid" 2>/dev/null; then
        echo -e "  ${RED}●${NC} $service - Not running (PID $pid not found)"
        rm -f "$PID_DIR/$service.pid"
        return 1
    fi
    
    # Check if service responds to health check
    local health_status
    local response_time
    local start_time=$(date +%s%N)
    
    if health_status=$(curl -s -f "http://localhost:$port/health" 2>/dev/null); then
        local end_time=$(date +%s%N)
        response_time=$(( (end_time - start_time) / 1000000 )) # Convert to milliseconds
        
        echo -e "  ${GREEN}●${NC} $service - Running (PID: $pid, Port: $port, Response: ${response_time}ms) ✓"
        
        # Record health metrics
        record_metric "$service" "response_time" "$response_time"
        record_metric "$service" "status" "healthy"
        
        status_code=0
    else
        echo -e "  ${YELLOW}●${NC} $service - Process exists but not responding (PID: $pid)"
        record_metric "$service" "status" "unhealthy"
    fi
    
    return $status_code
}

status_check() {
    log_info "Checking service status..."
    echo
    
    local healthy_count=0
    local total_count=${#SERVICES[@]}
    
    for service in "${!SERVICES[@]}"; do
        if [[ -d "$PROJECT_ROOT/$service" ]]; then
            if check_service_status "$service"; then
                ((healthy_count++))
            fi
        else
            log_debug "Service directory not found: $service (skipping from status check)"
            ((total_count--))
        fi
    done
    
    echo
    if [[ $healthy_count -eq $total_count ]]; then
        log_success "All $total_count services are running healthy"
    else
        log_warning "$healthy_count/$total_count services are healthy"
    fi
    
    # Check database connectivity
    echo
    log_info "Checking database connectivity..."
    if pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
        echo -e "  ${GREEN}●${NC} PostgreSQL - Connected ✓"
    else
        echo -e "  ${RED}●${NC} PostgreSQL - Connection failed ✗"
    fi
    
    # Check workspace status
    echo
    log_info "Checking Go workspace status..."
    if validate_workspace >/dev/null 2>&1; then
        echo -e "  ${GREEN}●${NC} Go workspace - Valid ✓"
    else
        echo -e "  ${YELLOW}●${NC} Go workspace - Issues detected ⚠"
    fi
    
    # Show metrics summary
    show_metrics_summary
}

show_metrics_summary() {
    if [[ -f "$METRICS_FILE" ]] && command -v jq &> /dev/null; then
        echo
        log_info "Performance metrics (last startup):"
        
        jq -r 'to_entries[] | select(.key | endswith("_startup_time")) | "  \(.key | gsub("_startup_time"; "")): \(.value)s"' "$METRICS_FILE" 2>/dev/null || true
    fi
}

# Performance monitoring and metrics
record_metric() {
    local service=$1
    local metric=$2
    local value=$3
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    if command -v jq &> /dev/null; then
        # Use jq if available for proper JSON handling
        local temp_file=$(mktemp)
        jq --arg service "$service" --arg metric "$metric" --arg value "$value" --arg timestamp "$timestamp" \
           '.[$service + "_" + $metric] = {value: $value, timestamp: $timestamp}' \
           "$METRICS_FILE" > "$temp_file" && mv "$temp_file" "$METRICS_FILE"
    else
        # Fallback to simple JSON append
        echo "{\"${service}_${metric}\": {\"value\": \"$value\", \"timestamp\": \"$timestamp\"}}" >> "$METRICS_FILE"
    fi
}

record_service_start() {
    local service=$1
    local pid=$2
    
    SERVICE_METRICS["${service}_start_time"]=$(date +%s)
    SERVICE_METRICS["${service}_pid"]=$pid
    
    record_metric "$service" "last_start" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
}

record_service_stop() {
    local service=$1
    
    if [[ -n "${SERVICE_METRICS["${service}_start_time"]:-}" ]]; then
        local start_time=${SERVICE_METRICS["${service}_start_time"]}
        local uptime=$(($(date +%s) - start_time))
        record_metric "$service" "last_uptime" "$uptime"
    fi
    
    record_metric "$service" "last_stop" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    
    unset SERVICE_METRICS["${service}_start_time"]
    unset SERVICE_METRICS["${service}_pid"]
}

# Enhanced backup operations
backup_databases() {
    log_info "Creating database backups..."
    
    if ! check_postgresql_status; then
        log_error "PostgreSQL is not running"
        return 1
    fi
    
    local backup_timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_subdir="$BACKUP_DIR/$backup_timestamp"
    
    mkdir -p "$backup_subdir"
    
    local success_count=0
    local total_count=${#DATABASES[@]}
    
    for db in "${DATABASES[@]}"; do
        log_info "Backing up database: $db"
        
        local backup_file="$backup_subdir/${db}.sql"
        
        # Try different backup methods
        if docker ps --format 'table {{.Names}}' | grep -q "accounting-postgres"; then
            # Docker container backup
            if docker exec accounting-postgres pg_dump -U "${DB_USER:-postgres}" "$db" > "$backup_file" 2>/dev/null; then
                log_success "Backed up $db (Docker)"
                ((success_count++))
            else
                log_error "Failed to backup $db (Docker)"
            fi
        elif sudo -u postgres pg_dump "$db" > "$backup_file" 2>/dev/null; then
            log_success "Backed up $db (system postgres)"
            ((success_count++))
        elif PGPASSWORD="${DB_PASSWORD}" pg_dump -h localhost -U "${DB_USER:-postgres}" "$db" > "$backup_file" 2>/dev/null; then
            log_success "Backed up $db (authenticated)"
            ((success_count++))
        else
            log_error "Failed to backup $db"
        fi
    done
    
    if [[ $success_count -eq $total_count ]]; then
        # Create compressed archive
        if tar -czf "$BACKUP_DIR/backup_${backup_timestamp}.tar.gz" -C "$BACKUP_DIR" "$backup_timestamp"; then
            log_success "Created compressed backup: backup_${backup_timestamp}.tar.gz"
            
            # Clean up individual SQL files
            rm -rf "$backup_subdir"
            
            # Clean old backups (keep last 30 days)
            find "$BACKUP_DIR" -name "backup_*.tar.gz" -mtime +30 -delete 2>/dev/null || true
            
            # Record backup metrics
            record_metric "system" "last_backup" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
            record_metric "system" "backup_size" "$(du -b "$BACKUP_DIR/backup_${backup_timestamp}.tar.gz" | cut -f1)"
            
            log_success "Database backup completed successfully"
        else
            log_error "Failed to create compressed backup"
            return 1
        fi
    else
        log_error "Backup incomplete: $success_count/$total_count databases backed up"
        return 1
    fi
}

# Enhanced log management
show_logs() {
    local service=$2
    local lines=${3:-50}
    local follow=${4:-false}
    
    if [[ -z "$service" ]]; then
        log_info "Available service logs:"
        if [[ -d "$LOG_DIR" ]]; then
            ls -la "$LOG_DIR/"
        else
            log_warning "No logs directory found"
        fi
        return 0
    fi
    
    local log_file="$LOG_DIR/$service.log"
    
    if [[ -f "$log_file" ]]; then
        log_info "Showing last $lines lines of $service logs:"
        echo "----------------------------------------"
        
        if [[ "$follow" == "true" ]]; then
            tail -f -n "$lines" "$log_file"
        else
            tail -n "$lines" "$log_file"
        fi
    else
        log_error "Log file not found: $log_file"
        return 1
    fi
}

# Testing support
run_tests() {
    local service=${1:-"all"}
    local type=${2:-"unit"}
    
    log_info "Running $type tests for: $service"
    
    cd "$PROJECT_ROOT"
    
    case "$type" in
        "unit")
            if [[ "$service" == "all" ]]; then
                go test ./... -short -v
            else
                go test "./$service/..." -short -v
            fi
            ;;
        "integration")
            if [[ "$service" == "all" ]]; then
                go test ./... -v -tags=integration
            else
                go test "./$service/..." -v -tags=integration
            fi
            ;;
        "e2e")
            run_e2e_tests
            ;;
        *)
            log_error "Unknown test type: $type"
            return 1
            ;;
    esac
}

run_e2e_tests() {
    log_info "Running end-to-end tests..."
    
    # Ensure all services are running
    if ! status_check >/dev/null 2>&1; then
        log_warning "Not all services are healthy. Starting services..."
        start_all_services
    fi
    
    # Run E2E tests if they exist
    if [[ -d "$PROJECT_ROOT/e2e" ]]; then
        cd "$PROJECT_ROOT/e2e"
        if [[ -f "package.json" ]]; then
            npm test
        else
            go test ./... -v -tags=e2e
        fi
    else
        log_warning "No E2E tests found"
    fi
}

# Development helpers
dev_reset() {
    log_warning "This will stop all services and reset the development environment"
    read -p "Are you sure? (y/N): " -n 1 -r
    echo
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Resetting development environment..."
        
        stop_all_services
        cleanup
        
        # Optionally recreate databases
        read -p "Reset databases as well? (y/N): " -n 1 -r
        echo
        
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            setup_database
        fi
        
        # Reset workspace
        if [[ -f "$WORKSPACE_FILE" ]]; then
            log_info "Resetting Go workspace..."
            rm -f "$WORKSPACE_FILE"
            setup_workspace
        fi
        
        log_success "Development environment reset completed"
    else
        log_info "Reset cancelled"
    fi
}

dev_watch() {
    local service=${1:-"all"}
    
    if ! command -v air &> /dev/null; then
        log_error "air not found. Install with: go install github.com/air-verse/air@latest"
        return 1
    fi
    
    if [[ "$service" == "all" ]]; then
        log_error "Cannot watch all services simultaneously. Please specify a service."
        return 1
    fi
    
    if [[ ! -d "$PROJECT_ROOT/$service" ]]; then
        log_error "Service directory not found: $service"
        return 1
    fi
    
    log_info "Starting $service in watch mode..."
    cd "$PROJECT_ROOT/$service"
    
    # Create .air.toml if it doesn't exist
    if [[ ! -f ".air.toml" ]]; then
        create_air_config "$service"
    fi
    
    air
}

create_air_config() {
    local service=$1
    local port=${SERVICES[$service]}
    
    cat > "$PROJECT_ROOT/$service/.air.toml" << EOF
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
args_bin = []
bin = "./tmp/main"
cmd = "go build -o ./tmp/main ."
delay = 1000
exclude_dir = ["assets", "tmp", "vendor", "testdata"]
exclude_file = []
exclude_regex = ["_test.go"]
exclude_unchanged = false
follow_symlink = false
full_bin = ""
include_dir = []
include_ext = ["go", "tpl", "tmpl", "html"]
include_file = []
kill_delay = "0s"
log = "build-errors.log"
rerun = false
rerun_delay = 500
send_interrupt = false
stop_on_root = false

[color]
app = ""
build = "yellow"
main = "magenta"
runner = "green"
watcher = "cyan"

[log]
main_only = false
time = false

[misc]
clean_on_exit = false

[screen]
clear_on_rebuild = false
keep_scroll = true
EOF
    
    log_debug "Created air configuration for $service"
}

# Enhanced cleanup operations
cleanup() {
    log_info "Cleaning up system..."
    
    stop_all_services
    
    # Clean up log files (keep last 7 days)
    find "$LOG_DIR" -name "*.log" -mtime +7 -delete 2>/dev/null || true
    
    # Clean up old PID files
    rm -f "$PID_DIR"/*.pid 2>/dev/null || true
    
    # Clean up temporary files
    find "$PROJECT_ROOT" -name "*.tmp" -delete 2>/dev/null || true
    find "$PROJECT_ROOT" -name ".DS_Store" -delete 2>/dev/null || true
    find "$PROJECT_ROOT" -name "tmp" -type d -exec rm -rf {} + 2>/dev/null || true
    
    # Clean up Go build cache
    go clean -cache -modcache 2>/dev/null || true
    
    # Clean up old metrics
    if [[ -f "$METRICS_FILE" ]] && command -v jq &> /dev/null; then
        # Keep only recent metrics (last 24 hours)
        local cutoff_time=$(date -u -d '24 hours ago' +"%Y-%m-%dT%H:%M:%SZ")
        jq --arg cutoff "$cutoff_time" 'to_entries | map(select(.value.timestamp > $cutoff)) | from_entries' "$METRICS_FILE" > "${METRICS_FILE}.tmp" && mv "${METRICS_FILE}.tmp" "$METRICS_FILE"
    fi
    
    log_success "Cleanup completed"
}

# Docker support
docker_build() {
    local service=${1:-"all"}
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker not found"
        return 1
    fi
    
    log_info "Building Docker images for: $service"
    
    if [[ "$service" == "all" ]]; then
        for svc in "${!SERVICES[@]}"; do
            if [[ -f "$PROJECT_ROOT/$svc/Dockerfile" ]]; then
                docker_build_service "$svc"
            fi
        done
    else
        docker_build_service "$service"
    fi
}

docker_build_service() {
    local service=$1
    
    if [[ ! -f "$PROJECT_ROOT/$service/Dockerfile" ]]; then
        log_warning "No Dockerfile found for $service"
        return 1
    fi
    
    log_info "Building Docker image for $service..."
    cd "$PROJECT_ROOT/$service"
    
    docker build -t "accounting-system/$service:latest" .
    
    if [[ $? -eq 0 ]]; then
        log_success "Docker image built for $service"
    else
        log_error "Failed to build Docker image for $service"
        return 1
    fi
}

docker_compose_up() {
    if [[ ! -f "$PROJECT_ROOT/docker-compose.yml" ]]; then
        log_error "No docker-compose.yml found"
        return 1
    fi
    
    log_info "Starting services with Docker Compose..."
    cd "$PROJECT_ROOT"
    
    docker-compose up -d
    
    log_success "Docker Compose services started"
}

docker_compose_down() {
    if [[ ! -f "$PROJECT_ROOT/docker-compose.yml" ]]; then
        log_error "No docker-compose.yml found"
        return 1
    fi
    
    log_info "Stopping Docker Compose services..."
    cd "$PROJECT_ROOT"
    
    docker-compose down
    
    log_success "Docker Compose services stopped"
}

# Configuration validation
validate_config() {
    log_info "Validating configuration..."
    
    validate_environment
    validate_workspace
    
    # Check service configurations
    for service in "${!SERVICES[@]}"; do
        if [[ -d "$PROJECT_ROOT/$service" ]]; then
            validate_service_config "$service"
        fi
    done
    
    log_success "Configuration validation completed"
}

validate_service_config() {
    local service=$1
    local service_dir="$PROJECT_ROOT/$service"
    
    # Check go.mod exists
    if [[ ! -f "$service_dir/go.mod" ]]; then
        log_warning "$service: Missing go.mod file"
    fi
    
    # Check main.go exists
    if [[ ! -f "$service_dir/main.go" ]]; then
        log_warning "$service: Missing main.go file"
    fi
    
    # Check Dockerfile if service should have one
    if [[ ! -f "$service_dir/Dockerfile" ]]; then
        log_debug "$service: No Dockerfile (not required for development)"
    fi
}

# Main command processing
print_usage() {
    echo -e "${WHITE}Indonesian Accounting System Management Script${NC}"
    echo
    echo -e "${CYAN}Usage:${NC} $0 {command} [options]"
    echo
    echo -e "${YELLOW}Setup Commands:${NC}"
    echo "  setup                    - Complete system setup (first-time installation)"
    echo "  install-deps            - Install project dependencies only"
    echo "  setup-db                - Setup databases only"
    echo "  setup-workspace         - Setup Go workspace"
    echo "  validate-config         - Validate configuration"
    echo
    echo -e "${YELLOW}Service Management:${NC}"
    echo "  start [service]         - Start all services or specific service"
    echo "  stop [service]          - Stop all services or specific service"
    echo "  restart [service]       - Restart all services or specific service"
    echo "  status                  - Check service status with metrics"
    echo
    echo -e "${YELLOW}Development:${NC}"
    echo "  watch <service>         - Start service in watch mode (hot reload)"
    echo "  test [service] [type]   - Run tests (unit/integration/e2e)"
    echo "  dev-reset              - Reset development environment"
    echo "  sync-workspace         - Sync Go workspace dependencies"
    echo
    echo -e "${YELLOW}Database:${NC}"
    echo "  migrate [service]       - Run database migrations"
    echo "  backup                  - Backup all databases"
    echo
    echo -e "${YELLOW}Docker:${NC}"
    echo "  docker-build [service]  - Build Docker images"
    echo "  docker-up              - Start with Docker Compose"
    echo "  docker-down            - Stop Docker Compose"
    echo
    echo -e "${YELLOW}Maintenance:${NC}"
    echo "  logs [service] [lines] [follow] - Show logs"
    echo "  clean                   - Clean temporary files and old logs"
    echo "  metrics                 - Show performance metrics"
    echo
    echo -e "${YELLOW}Available services:${NC}"
    printf "  %s\n" "${!SERVICES[@]}" | sort
    echo
    echo -e "${YELLOW}Examples:${NC}"
    echo "  $0 setup                # Initial system setup"
    echo "  $0 start                # Start all services"
    echo "  $0 start user-service   # Start only user service"
    echo "  $0 watch api-gateway    # Watch API gateway with hot reload"
    echo "  $0 logs api-gateway 100 true  # Follow last 100 lines"
    echo "  $0 test user-service unit      # Run unit tests"
    echo "  $0 status               # Check all service status"
}

# Main execution
main() {
    local command=${1:-""}
    local arg1=${2:-""}
    local arg2=${3:-""}
    local arg3=${4:-""}
    
    # Change to project root directory
    cd "$PROJECT_ROOT"
    
    # Load environment if available
    if [[ -f ".env" ]]; then
        set -a
        source .env 2>/dev/null || true
        set +a
    fi
    
    case "$command" in
        setup)
            log_info "Starting complete system setup..."
            check_requirements || exit 1
            setup_environment || exit 1
            install_dependencies || exit 1
            setup_database || exit 1
            validate_config || exit 1
            log_success "System setup completed successfully!"
            log_info "Run '$0 start' to start all services"
            ;;
            
        install-deps)
            install_dependencies || exit 1
            ;;
            
        setup-db)
            setup_database || exit 1
            ;;
            
        setup-workspace)
            setup_workspace || exit 1
            ;;
            
        sync-workspace)
            sync_workspace || exit 1
            ;;
            
        validate-config)
            validate_config || exit 1
            ;;
            
        start)
            if [[ -n "$arg1" ]]; then
                start_service "$arg1" || exit 1
            else
                start_all_services || exit 1
            fi
            ;;
            
        stop)
            if [[ -n "$arg1" ]]; then
                stop_service "$arg1" || exit 1
            else
                stop_all_services || exit 1
            fi
            ;;
            
        restart)
            restart_service "$arg1" || exit 1
            ;;
            
        status)
            status_check
            ;;
            
        watch)
            if [[ -z "$arg1" ]]; then
                log_error "Service name required for watch command"
                exit 1
            fi
            dev_watch "$arg1" || exit 1
            ;;
            
        test)
            run_tests "$arg1" "$arg2" || exit 1
            ;;
            
        migrate)
            run_migrations "$arg1" || exit 1
            ;;
            
        backup)
            backup_databases || exit 1
            ;;
            
        docker-build)
            docker_build "$arg1" || exit 1
            ;;
            
        docker-up)
            docker_compose_up || exit 1
            ;;
            
        docker-down)
            docker_compose_down || exit 1
            ;;
            
        logs)
            show_logs "$command" "$arg1" "${arg2:-50}" "${arg3:-false}"
            ;;
            
        clean)
            cleanup
            ;;
            
        metrics)
            show_metrics_summary
            ;;
            
        dev-reset)
            dev_reset
            ;;
            
        *)
            print_usage
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"