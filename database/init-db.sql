-- Enhanced Database Initialization Script with Company Service Support

-- Create all databases (including company_db)
CREATE DATABASE user_db;
CREATE DATABASE company_db;
CREATE DATABASE account_db;
CREATE DATABASE transaction_db;
CREATE DATABASE invoice_db;
CREATE DATABASE vendor_db;
CREATE DATABASE inventory_db;
CREATE DATABASE tax_db;

-- User Database Setup
\c user_db;

CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    tax_id VARCHAR(50) UNIQUE NOT NULL,
    address TEXT,
    phone VARCHAR(20),
    email VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_tax_id_format CHECK (tax_id ~ '^\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}$')
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL CHECK (role IN ('admin', 'manager', 'accountant', 'user')),
    company_id INTEGER REFERENCES companies(id) ON DELETE CASCADE,
    is_active BOOLEAN DEFAULT TRUE,
    last_login TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Enhanced audit log table
CREATE TABLE audit_log (
    id SERIAL PRIMARY KEY,
    table_name VARCHAR(50) NOT NULL,
    record_id INTEGER,
    operation VARCHAR(10) NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    user_id INTEGER REFERENCES users(id),
    old_values JSONB,
    new_values JSONB,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ip_address INET,
    user_agent TEXT
);

-- Insert sample data
INSERT INTO companies (name, tax_id, address, phone, email) VALUES 
('PT Contoh Indonesia', '01.234.567.8-901.000', 'Jakarta, Indonesia', '+62-21-1234567', 'admin@contoh.co.id');

-- Password: 'password123' (use bcrypt with cost 12 in production)
INSERT INTO users (email, password_hash, name, role, company_id) VALUES 
('admin@contoh.co.id', '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj1h4yOy.Z7C', 'Administrator', 'admin', 1),
('manager@contoh.co.id', '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj1h4yOy.Z7C', 'Manager', 'manager', 1),
('accountant@contoh.co.id', '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj1h4yOy.Z7C', 'Accountant', 'accountant', 1);

-- Company Database Setup (NEW)
\c company_db;

CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    tax_id VARCHAR(50) UNIQUE NOT NULL,
    address TEXT,
    phone VARCHAR(20),
    email VARCHAR(255),
    business_type VARCHAR(100),
    registration_date DATE,
    fiscal_year_end DATE DEFAULT '12-31',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_tax_id_format CHECK (tax_id ~ '^\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}$'),
    CONSTRAINT check_email_format CHECK (email ~ '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

-- Company settings for Indonesian compliance
CREATE TABLE company_settings (
    id SERIAL PRIMARY KEY,
    company_id INTEGER REFERENCES companies(id) ON DELETE CASCADE,
    setting_key VARCHAR(100) NOT NULL,
    setting_value TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, setting_key)
);

-- Insert sample company data
INSERT INTO companies (name, tax_id, address, phone, email, business_type) VALUES 
('PT Contoh Indonesia', '01.234.567.8-901.000', 'Jakarta, Indonesia', '+62-21-1234567', 'admin@contoh.co.id', 'Technology Services');

-- Insert Indonesian compliance settings
INSERT INTO company_settings (company_id, setting_key, setting_value) VALUES 
(1, 'default_currency', 'IDR'),
(1, 'default_timezone', 'Asia/Jakarta'),
(1, 'tax_rate_ppn', '11.00'),
(1, 'fiscal_year_start', '01-01'),
(1, 'reporting_language', 'id-ID');

-- Account Database Setup
\c account_db;

CREATE TABLE chart_of_accounts (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    account_code VARCHAR(20) NOT NULL,
    account_name VARCHAR(255) NOT NULL,
    account_type VARCHAR(50) NOT NULL CHECK (account_type IN ('Asset', 'Liability', 'Equity', 'Revenue', 'Expense')),
    parent_id INTEGER REFERENCES chart_of_accounts(id),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, account_code),
    CONSTRAINT check_account_code_format CHECK (account_code ~ '^\d{4}$')
);

CREATE TABLE general_ledger (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    account_id INTEGER REFERENCES chart_of_accounts(id),
    transaction_date DATE NOT NULL,
    description TEXT NOT NULL,
    debit_amount DECIMAL(15,0) DEFAULT 0 CHECK (debit_amount >= 0),
    credit_amount DECIMAL(15,0) DEFAULT 0 CHECK (credit_amount >= 0),
    reference_id VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_debit_or_credit CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR 
        (debit_amount = 0 AND credit_amount > 0)
    ),
    CONSTRAINT check_idr_amounts CHECK (
        debit_amount = ROUND(debit_amount) AND credit_amount = ROUND(credit_amount)
    )
);

-- Insert Indonesian chart of accounts
INSERT INTO chart_of_accounts (company_id, account_code, account_name, account_type, is_active) VALUES 
-- Assets
(1, '1000', 'Kas', 'Asset', true),
(1, '1100', 'Piutang Usaha', 'Asset', true),
(1, '1200', 'Persediaan', 'Asset', true),
(1, '1300', 'Biaya Dibayar Dimuka', 'Asset', true),
(1, '1400', 'Aset Tetap', 'Asset', true),
(1, '1500', 'Akumulasi Penyusutan', 'Asset', true),
-- Liabilities
(1, '2000', 'Utang Usaha', 'Liability', true),
(1, '2100', 'Biaya Yang Masih Harus Dibayar', 'Liability', true),
(1, '2200', 'Utang Jangka Pendek', 'Liability', true),
(1, '2300', 'Utang Jangka Panjang', 'Liability', true),
(1, '2400', 'Utang PPN', 'Liability', true),
-- Equity
(1, '3000', 'Modal Saham', 'Equity', true),
(1, '3100', 'Laba Ditahan', 'Equity', true),
(1, '3200', 'Laba Tahun Berjalan', 'Equity', true),
-- Revenue
(1, '4000', 'Pendapatan Penjualan', 'Revenue', true),
(1, '4100', 'Pendapatan Jasa', 'Revenue', true),
(1, '4200', 'Pendapatan Lain-lain', 'Revenue', true),
-- Expenses
(1, '5000', 'Harga Pokok Penjualan', 'Expense', true),
(1, '5100', 'Biaya Operasional', 'Expense', true),
(1, '5200', 'Biaya Penyusutan', 'Expense', true),
(1, '5300', 'Biaya Bunga', 'Expense', true),
(1, '5400', 'Biaya Pajak', 'Expense', true);

-- Transaction Database Setup
\c transaction_db;

CREATE TABLE journal_entries (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    entry_number VARCHAR(50) NOT NULL,
    entry_date DATE NOT NULL,
    description TEXT NOT NULL,
    total_amount DECIMAL(15,0) NOT NULL CHECK (total_amount >= 0),
    status VARCHAR(20) DEFAULT 'draft' CHECK (status IN ('draft', 'posted', 'cancelled', 'reversed')),
    created_by INTEGER NOT NULL,
    posted_by INTEGER,
    posted_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, entry_number),
    CONSTRAINT check_idr_total_amount CHECK (total_amount = ROUND(total_amount))
);

CREATE TABLE journal_entry_lines (
    id SERIAL PRIMARY KEY,
    journal_entry_id INTEGER REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL,
    description TEXT,
    debit_amount DECIMAL(15,0) DEFAULT 0 CHECK (debit_amount >= 0),
    credit_amount DECIMAL(15,0) DEFAULT 0 CHECK (credit_amount >= 0),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_debit_or_credit CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR 
        (debit_amount = 0 AND credit_amount > 0)
    ),
    CONSTRAINT check_idr_line_amounts CHECK (
        debit_amount = ROUND(debit_amount) AND credit_amount = ROUND(credit_amount)
    )
);

-- Invoice Database Setup
\c invoice_db;

CREATE TABLE customers (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    customer_code VARCHAR(20) NOT NULL,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(20),
    address TEXT,
    tax_id VARCHAR(50),
    payment_terms INTEGER DEFAULT 30 CHECK (payment_terms >= 0 AND payment_terms <= 365),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, customer_code),
    CONSTRAINT check_customer_tax_id CHECK (tax_id IS NULL OR tax_id ~ '^\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}$')
);

CREATE TABLE invoices (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    customer_id INTEGER REFERENCES customers(id),
    invoice_number VARCHAR(50) NOT NULL,
    invoice_date DATE NOT NULL,
    due_date DATE NOT NULL,
    subtotal DECIMAL(15,0) NOT NULL CHECK (subtotal >= 0),
    tax_amount DECIMAL(15,0) DEFAULT 0 CHECK (tax_amount >= 0),
    total_amount DECIMAL(15,0) NOT NULL CHECK (total_amount >= 0),
    status VARCHAR(20) DEFAULT 'draft' CHECK (status IN ('draft', 'sent', 'paid', 'overdue', 'cancelled')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, invoice_number),
    CONSTRAINT check_idr_invoice_amounts CHECK (
        subtotal = ROUND(subtotal) AND 
        tax_amount = ROUND(tax_amount) AND 
        total_amount = ROUND(total_amount)
    )
);

CREATE TABLE invoice_lines (
    id SERIAL PRIMARY KEY,
    invoice_id INTEGER REFERENCES invoices(id) ON DELETE CASCADE,
    product_name VARCHAR(255) NOT NULL,
    quantity DECIMAL(10,2) NOT NULL CHECK (quantity > 0),
    unit_price DECIMAL(15,0) NOT NULL CHECK (unit_price >= 0),
    line_total DECIMAL(15,0) NOT NULL CHECK (line_total >= 0),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_idr_line_amounts CHECK (
        unit_price = ROUND(unit_price) AND line_total = ROUND(line_total)
    )
);

-- Insert sample customers
INSERT INTO customers (company_id, customer_code, name, email, phone, address, tax_id) VALUES 
(1, 'CUST001', 'PT Mitra Bisnis', 'mitra@bisnis.co.id', '+62-21-1234567', 'Jakarta', '01.234.567.8-901.001'),
(1, 'CUST002', 'CV Sejahtera', 'info@sejahtera.co.id', '+62-21-1234568', 'Bandung', '01.234.567.8-901.002'),
(1, 'CUST003', 'PT Global Tech', 'admin@globaltech.co.id', '+62-21-1234569', 'Surabaya', '01.234.567.8-901.003');

-- Vendor Database Setup
\c vendor_db;

CREATE TABLE vendors (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    vendor_code VARCHAR(20) NOT NULL,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(20),
    address TEXT,
    tax_id VARCHAR(50),
    payment_terms INTEGER DEFAULT 30 CHECK (payment_terms >= 0 AND payment_terms <= 365),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, vendor_code),
    CONSTRAINT check_vendor_tax_id CHECK (tax_id IS NULL OR tax_id ~ '^\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}$')
);

CREATE TABLE purchase_orders (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    vendor_id INTEGER REFERENCES vendors(id),
    po_number VARCHAR(50) NOT NULL,
    order_date DATE NOT NULL,
    expected_date DATE,
    subtotal DECIMAL(15,0) NOT NULL CHECK (subtotal >= 0),
    tax_amount DECIMAL(15,0) DEFAULT 0 CHECK (tax_amount >= 0),
    total_amount DECIMAL(15,0) NOT NULL CHECK (total_amount >= 0),
    status VARCHAR(20) DEFAULT 'draft' CHECK (status IN ('draft', 'sent', 'confirmed', 'delivered', 'cancelled')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, po_number),
    CONSTRAINT check_idr_po_amounts CHECK (
        subtotal = ROUND(subtotal) AND 
        tax_amount = ROUND(tax_amount) AND 
        total_amount = ROUND(total_amount)
    )
);

-- Insert sample vendors
INSERT INTO vendors (company_id, vendor_code, name, email, phone, address, tax_id, payment_terms) VALUES 
(1, 'VEND001', 'PT Supplier Utama', 'supplier@utama.co.id', '+62-21-2345678', 'Jakarta', '01.234.567.8-902.001', 30),
(1, 'VEND002', 'CV Distributor Prima', 'info@distributor.co.id', '+62-21-2345679', 'Surabaya', '01.234.567.8-902.002', 15),
(1, 'VEND003', 'PT Office Supply', 'sales@office.co.id', '+62-21-2345680', 'Bandung', '01.234.567.8-902.003', 45);

-- Inventory Database Setup
\c inventory_db;

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    product_code VARCHAR(50) NOT NULL,
    product_name VARCHAR(255) NOT NULL,
    description TEXT,
    unit_price DECIMAL(15,0) NOT NULL CHECK (unit_price >= 0),
    cost_price DECIMAL(15,0) NOT NULL CHECK (cost_price >= 0),
    quantity_on_hand INTEGER DEFAULT 0 CHECK (quantity_on_hand >= 0),
    minimum_stock INTEGER DEFAULT 0 CHECK (minimum_stock >= 0),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company_id, product_code),
    CONSTRAINT check_idr_product_amounts CHECK (
        unit_price = ROUND(unit_price) AND cost_price = ROUND(cost_price)
    )
);

CREATE TABLE stock_movements (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    product_id INTEGER REFERENCES products(id),
    movement_type VARCHAR(20) NOT NULL CHECK (movement_type IN ('IN', 'OUT', 'ADJUSTMENT_IN', 'ADJUSTMENT_OUT', 'TRANSFER')),
    quantity INTEGER NOT NULL,
    unit_cost DECIMAL(15,0),
    reference_number VARCHAR(100),
    movement_date DATE NOT NULL,
    notes TEXT,
    created_by INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_idr_unit_cost CHECK (unit_cost IS NULL OR unit_cost = ROUND(unit_cost))
);

-- Insert sample products
INSERT INTO products (company_id, product_code, product_name, description, unit_price, cost_price, quantity_on_hand, minimum_stock) VALUES 
(1, 'PROD001', 'Laptop Dell Inspiron', 'Dell Inspiron 15 3000 Series', 8000000, 6500000, 10, 5),
(1, 'PROD002', 'Mouse Wireless Logitech', 'Logitech M705 Marathon Mouse', 350000, 250000, 25, 10),
(1, 'PROD003', 'Keyboard Mechanical', 'Mechanical Gaming Keyboard RGB', 750000, 500000, 15, 8),
(1, 'SERV001', 'IT Consultation', 'Hourly IT consultation service', 500000, 300000, 0, 0),
(1, 'SERV002', 'System Maintenance', 'Monthly system maintenance service', 2000000, 1200000, 0, 0);

-- Tax Database Setup
\c tax_db;

CREATE TABLE tax_rates (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    tax_name VARCHAR(100) NOT NULL,
    tax_rate DECIMAL(5,2) NOT NULL CHECK (tax_rate >= 0 AND tax_rate <= 100),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tax_transactions (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    transaction_id INTEGER NOT NULL,
    transaction_type VARCHAR(20) NOT NULL CHECK (transaction_type IN ('INVOICE', 'PURCHASE', 'JOURNAL')),
    tax_rate_id INTEGER REFERENCES tax_rates(id),
    tax_base DECIMAL(15,0) NOT NULL CHECK (tax_base >= 0),
    tax_amount DECIMAL(15,0) NOT NULL CHECK (tax_amount >= 0),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT check_idr_tax_amounts CHECK (
        tax_base = ROUND(tax_base) AND tax_amount = ROUND(tax_amount)
    )
);

-- Insert Indonesian tax rates
INSERT INTO tax_rates (company_id, tax_name, tax_rate, is_active) VALUES 
(1, 'PPN (Pajak Pertambahan Nilai)', 11.00, true),
(1, 'PPh 21 (Pajak Penghasilan Pasal 21)', 5.00, true),
(1, 'PPh 23 (Pajak Penghasilan Pasal 23)', 2.00, true),
(1, 'PPh 4(2) Final', 0.50, true),
(1, 'PPh Badan', 25.00, true);

-- CREATE ENHANCED INDEXES FOR PERFORMANCE
\c user_db;
CREATE INDEX idx_users_company_email ON users(company_id, email);
CREATE INDEX idx_users_active ON users(is_active) WHERE is_active = true;
CREATE INDEX idx_audit_log_table_record ON audit_log(table_name, record_id);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX idx_companies_tax_id ON companies(tax_id);

\c company_db;
CREATE INDEX idx_companies_tax_id ON companies(tax_id);
CREATE INDEX idx_company_settings_key ON company_settings(company_id, setting_key);

\c account_db;
CREATE INDEX idx_accounts_company_type ON chart_of_accounts(company_id, account_type);
CREATE INDEX idx_accounts_active ON chart_of_accounts(company_id, is_active) WHERE is_active = true;
CREATE INDEX idx_ledger_account_date ON general_ledger(account_id, transaction_date);
CREATE INDEX idx_ledger_company_date ON general_ledger(company_id, transaction_date);
CREATE INDEX idx_ledger_reference ON general_ledger(reference_id) WHERE reference_id IS NOT NULL;

\c transaction_db;
CREATE INDEX idx_transactions_company_date ON journal_entries(company_id, entry_date);
CREATE INDEX idx_transactions_status ON journal_entries(company_id, status);
CREATE INDEX idx_transaction_lines_entry ON journal_entry_lines(journal_entry_id);

\c invoice_db;
CREATE INDEX idx_invoices_company_status ON invoices(company_id, status);
CREATE INDEX idx_invoices_date ON invoices(company_id, invoice_date);
CREATE INDEX idx_invoices_due_date ON invoices(due_date) WHERE status IN ('sent', 'overdue');
CREATE INDEX idx_customers_company_active ON customers(company_id, is_active) WHERE is_active = true;
CREATE INDEX idx_invoice_lines_invoice ON invoice_lines(invoice_id);

\c vendor_db;
CREATE INDEX idx_vendors_company_active ON vendors(company_id, is_active) WHERE is_active = true;
CREATE INDEX idx_purchase_orders_company_status ON purchase_orders(company_id, status);
CREATE INDEX idx_purchase_orders_date ON purchase_orders(company_id, order_date);

\c inventory_db;
CREATE INDEX idx_products_company_active ON products(company_id, is_active) WHERE is_active = true;
CREATE INDEX idx_products_low_stock ON products(company_id) WHERE quantity_on_hand <= minimum_stock AND is_active = true;
CREATE INDEX idx_stock_movements_product_date ON stock_movements(product_id, movement_date);
CREATE INDEX idx_stock_movements_company_date ON stock_movements(company_id, movement_date);

\c tax_db;
CREATE INDEX idx_tax_rates_company_active ON tax_rates(company_id, is_active) WHERE is_active = true;
CREATE INDEX idx_tax_transactions_company_type ON tax_transactions(company_id, transaction_type);

-- Create functions for automatic updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create audit logging function
CREATE OR REPLACE FUNCTION audit_log_changes()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO audit_log (table_name, record_id, operation, old_values, new_values)
    VALUES (
        TG_TABLE_NAME,
        COALESCE(NEW.id, OLD.id),
        TG_OP,
        CASE WHEN TG_OP = 'DELETE' THEN row_to_json(OLD) ELSE NULL END,
        CASE WHEN TG_OP = 'INSERT' OR TG_OP = 'UPDATE' THEN row_to_json(NEW) ELSE NULL END
    );
    RETURN COALESCE(NEW, OLD);
END;
$$ language 'plpgsql';

-- Apply triggers to tables with updated_at columns
\c user_db;
CREATE TRIGGER update_companies_updated_at BEFORE UPDATE ON companies FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER audit_companies AFTER INSERT OR UPDATE OR DELETE ON companies FOR EACH ROW EXECUTE FUNCTION audit_log_changes();
CREATE TRIGGER audit_users AFTER INSERT OR UPDATE OR DELETE ON users FOR EACH ROW EXECUTE FUNCTION audit_log_changes();

\c company_db;
CREATE TRIGGER update_companies_updated_at BEFORE UPDATE ON companies FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_company_settings_updated_at BEFORE UPDATE ON company_settings FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

\c account_db;
CREATE TRIGGER update_accounts_updated_at BEFORE UPDATE ON chart_of_accounts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

\c transaction_db;
CREATE TRIGGER update_journal_entries_updated_at BEFORE UPDATE ON journal_entries FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER audit_journal_entries AFTER INSERT OR UPDATE OR DELETE ON journal_entries FOR EACH ROW EXECUTE FUNCTION audit_log_changes();

\c invoice_db;
CREATE TRIGGER update_customers_updated_at BEFORE UPDATE ON customers FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_invoices_updated_at BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

\c vendor_db;
CREATE TRIGGER update_vendors_updated_at BEFORE UPDATE ON vendors FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_purchase_orders_updated_at BEFORE UPDATE ON purchase_orders FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

\c inventory_db;
CREATE TRIGGER update_products_updated_at BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

\c tax_db;
CREATE TRIGGER update_tax_rates_updated_at BEFORE UPDATE ON tax_rates FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();