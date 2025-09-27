// frontend/src/services/api.js - SIMPLIFIED VERSION
import axios from 'axios';

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost:8000/api';

class ApiService {
    constructor() {
        this.api = axios.create({
            baseURL: API_BASE_URL,
            timeout: 30000,
            headers: {
                'Content-Type': 'application/json',
            }
        });

        this.setupInterceptors();
    }

    setupInterceptors() {
        // Request interceptor
        this.api.interceptors.request.use(
            (config) => {
                const token = localStorage.getItem('token');
                if (token) {
                    config.headers.Authorization = `Bearer ${token}`;
                }
                return config;
            },
            (error) => Promise.reject(error)
        );

        // Response interceptor
        this.api.interceptors.response.use(
            (response) => response,
            (error) => {
                if (error.response?.status === 401) {
                    this.clearAuth();
                    window.location.href = '/login';
                }
                return Promise.reject(error);
            }
        );
    }

    // Authentication
    async login(credentials) {
        const response = await this.api.post('/auth/login', credentials);
        const { data } = response.data;
        
        localStorage.setItem('token', data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        localStorage.setItem('company', JSON.stringify(data.company));
        
        return response.data;
    }

    // Generic CRUD operations
    async get(endpoint, params = {}) {
        return this.api.get(endpoint, { params });
    }

    async post(endpoint, data) {
        return this.api.post(endpoint, data);
    }

    async put(endpoint, data) {
        return this.api.put(endpoint, data);
    }

    async delete(endpoint) {
        return this.api.delete(endpoint);
    }

    // Specific API methods
    async getAccounts(companyId) {
        return this.get('/accounts', { company_id: companyId });
    }

    async createAccount(account) {
        return this.post('/accounts', account);
    }

    async getTransactions(companyId) {
        return this.get('/transactions', { company_id: companyId });
    }

    async createTransaction(transaction) {
        return this.post('/transactions', transaction);
    }

    async getInvoices(companyId) {
        return this.get('/invoices', { company_id: companyId });
    }

    async createInvoice(invoice) {
        return this.post('/invoices', invoice);
    }

    async getCustomers(companyId) {
        return this.get('/customers', { company_id: companyId });
    }

    clearAuth() {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        localStorage.removeItem('company');
    }

    getCurrentUser() {
        try {
            const userStr = localStorage.getItem('user');
            return userStr ? JSON.parse(userStr) : null;
        } catch (error) {
            this.clearAuth();
            return null;
        }
    }
}

export default new ApiService();