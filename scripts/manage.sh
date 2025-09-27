#!/bin/bash
# scripts/manage.sh - Enhanced version with complete company service support

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

# Service definitions with enhanced metadata (FIXED: Added company-service)
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
    ["api-gateway"]="user-service account-service company-service"
    ["transaction-service"]="account-service"
    ["report-service"]="account-service transaction-service"
    ["invoice-service"]="user-service company-service"
    ["vendor-service"]="user-service company-service"
    ["inventory-service"]="user-service company-service"
    ["account-service"]="company-service"
)

readonly DATABASES=("user_db" "company_db" "account_db" "transaction_db" "invoice_db" "vendor_db" "inventory_db" "tax_db")
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

# Enhanced Indonesian compliance validation
validate_indonesian_compliance() {
    log_info "Validating Indonesian business compliance..."
    
    local warnings=()
    local errors=()
    
    # Check timezone
    if [[ "$DEFAULT_TIMEZONE" != "Asia/Jakarta" ]]; then
        warnings+=("Timezone should be Asia/Jakarta for Indonesian compliance")
    fi
    
    # Validate PPN rate
    if [[ "$TAX_RATE_PPN" != "11.00" ]]; then
        warnings+=("PPN rate should be 11% for current Indonesian tax law")
    fi
    
    # Check currency
    if [[ "$DEFAULT_CURRENCY" != "IDR" ]]; then
        warnings+=("Currency should be IDR for Indonesian business")
    fi
    
    # Validate JWT secret for Indonesian compliance (should be strong)
    if [[ ${#JWT_SECRET} -lt 64 ]]; then
        errors+=("JWT_SECRET should be at least 64 characters for production security")
    fi
    
    # Check database password strength
    if [[ ${#DB_PASSWORD} -lt 16 ]]; then
        warnings+=("Database password should be at least 16 characters for security")
    fi
    
    # Report findings
    if [[ ${#errors[@]} -gt 0 ]]; then
        log_error "Indonesian compliance errors found:"
        printf "  - %s\n" "${errors[@]}"
        return 1
    fi
    
    if [[ ${#warnings[@]} -gt 0 ]]; then
        log_warning "Indonesian compliance warnings:"
        printf "  - %s\n" "${warnings[@]}"
    fi
    
    log_success "Indonesian compliance validation completed"
    return 0
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
        
        # Add all services (including company-service)
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
    
    # Check if all services are in workspace (including company-service)
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

# Enhanced environment setup with Indonesian compliance
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
    
    # Validate environment (including Indonesian compliance)
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
JWT_SECRET=your-super-secure-jwt-secret-key-must-be-at-least-64-characters-long-for-production-security
JWT_EXPIRATION=86400

# Session Security
SESSION_SECRET=your-session-secret-key-must-be-at-least-64-characters-long-for-production-security
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

# Service URLs
USER_SERVICE_URL=http://localhost:8001
COMPANY_SERVICE_URL=http://localhost:8011
ACCOUNT_SERVICE_URL=http://localhost:8002
TRANSACTION_SERVICE_URL=http://localhost:8003
INVOICE_SERVICE_URL=http://localhost:8004
VENDOR_SERVICE_URL=http://localhost:8005
INVENTORY_SERVICE_URL=http://localhost:8006
REPORT_SERVICE_URL=http://localhost:8007
TAX_SERVICE_URL=http://localhost:8008
CURRENCY_SERVICE_URL=http://localhost:8009
NOTIFICATION_SERVICE_URL=http://localhost:8010

# Frontend Configuration
REACT_APP_API_URL=http://localhost:8000/api
FRONTEND_URL=http://localhost:3000

# Development Environment
NODE_ENV=development
GO_ENV=development

# Debug Mode
DEBUG=false

# Optional Services
SMTP_HOST=
SMTP_USER=
SMTP_PASSWORD=
EXCHANGE_API_KEY=
REDIS_PASSWORD=
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
    
    # Validate Indonesian business settings and overall compliance
    validate_indonesian_compliance
    
    log_success "Environment validation passed"
    return 0
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

start_all_services() {
    log_info "Starting Indonesian Accounting System services..."
    
    if ! check_postgresql_status; then
        if ! start_postgresql; then
            log_error "Cannot start services without PostgreSQL"
            return 1
        fi
    fi
    
    setup_directories
    
    # Start services in dependency order (including company-service)
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
    echo -e "  ${CYAN}API Docs:${NC}        http://localhost:8000/api/docs"
    echo -e "  ${CYAN}Frontend:${NC}        http://localhost:3000 (run 'npm start' in frontend directory)"
    echo
    log_info "🏢 Indonesian Business Services:"
    echo -e "  ${CYAN}Company Mgmt:${NC}    http://localhost:8011/health"
    echo -e "  ${CYAN}Tax Service:${NC}     http://localhost:8008/health" 
    echo -e "  ${CYAN}Currency:${NC}        http://localhost:8009/health"
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

# [Rest of the functions remain the same as the original file, including:]
# - stop_service, stop_all_services, restart_service, restart_all_services
# - check_service_status, status_check, show_metrics_summary
# - record_metric, record_service_start, record_service_stop
# - backup_databases, show_logs, run_tests, dev_reset, dev_watch
# - cleanup, docker operations, configuration validation
# - main function and all helper functions

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

# [Include all other functions from the original manage.sh file]
# This includes PostgreSQL management, database operations, testing, etc.
# For brevity, I'm not repeating all functions, but they should all be included

# Main command processing remains the same
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
            log_info "Starting complete system setup with Indonesian compliance..."
            check_requirements || exit 1
            setup_environment || exit 1
            install_dependencies || exit 1
            setup_database || exit 1
            validate_config || exit 1
            validate_indonesian_compliance || log_warning "Indonesian compliance issues detected"
            log_success "System setup completed successfully!"
            log_info "Run '$0 start' to start all services"
            ;;
        # [All other cases remain the same as original]
        *)
            print_usage
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"