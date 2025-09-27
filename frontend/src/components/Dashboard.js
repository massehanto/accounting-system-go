// frontend/src/components/Dashboard.js - OPTIMIZED VERSION
import React, { useState, useEffect } from 'react';
import {
  Container, Grid, Paper, Typography, Box, Card, CardContent,
  Alert, Chip, Button, CircularProgress
} from '@mui/material';
import {
  TrendingUp, AccountBalance, Receipt, Warning, Refresh
} from '@mui/icons-material';
import { useAuth } from '../contexts/AuthContext';
import { useNotification } from '../contexts/NotificationContext';
import ApiService from '../services/api';

const Dashboard = () => {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const { user } = useAuth();
  const { showError } = useNotification();

  useEffect(() => {
    if (user?.company_id) {
      fetchDashboardData();
    }
  }, [user]);

  const fetchDashboardData = async () => {
    try {
      setLoading(true);
      
      const [accountsRes, transactionsRes, invoicesRes] = await Promise.allSettled([
        ApiService.getAccounts(user.company_id),
        ApiService.getTransactions(user.company_id),
        ApiService.getInvoices(user.company_id)
      ]);

      const accounts = accountsRes.status === 'fulfilled' ? accountsRes.value.data.data || [] : [];
      const transactions = transactionsRes.status === 'fulfilled' ? transactionsRes.value.data.data || [] : [];
      const invoices = invoicesRes.status === 'fulfilled' ? invoicesRes.value.data.data || [] : [];

      setStats({
        totalAccounts: accounts.length,
        totalTransactions: transactions.length,
        totalInvoices: invoices.length,
        totalRevenue: invoices.reduce((sum, inv) => sum + (parseFloat(inv.total_amount) || 0), 0),
        pendingInvoices: invoices.filter(inv => ['sent', 'pending'].includes(inv.status)).length,
        draftTransactions: transactions.filter(tx => tx.status === 'draft').length,
        overdueInvoices: invoices.filter(inv => {
          if (!inv.due_date || inv.status === 'paid') return false;
          return new Date(inv.due_date) < new Date();
        }).length
      });
    } catch (error) {
      showError(error.message || 'Failed to load dashboard data');
    } finally {
      setLoading(false);
    }
  };

  const StatCard = ({ title, value, icon, color, subtitle, alert }) => (
    <Card>
      <CardContent>
        <Box display="flex" alignItems="center" justifyContent="space-between">
          <Box>
            <Typography color="textSecondary" gutterBottom variant="body2">
              {title}
            </Typography>
            <Typography variant="h4">
              {typeof value === 'number' && title.includes('Revenue') 
                ? `Rp ${value.toLocaleString('id-ID')}` 
                : value}
            </Typography>
            {subtitle && (
              <Typography variant="body2" color="textSecondary">
                {subtitle}
              </Typography>
            )}
            {alert && (
              <Chip 
                label={alert} 
                color={alert.includes('Review') ? 'warning' : 'error'} 
                size="small" 
                sx={{ mt: 1 }} 
              />
            )}
          </Box>
          <Box color={color} sx={{ opacity: 0.7 }}>
            {icon}
          </Box>
        </Box>
      </CardContent>
    </Card>
  );

  if (loading) {
    return (
      <Container maxWidth="lg" sx={{ mt: 4, display: 'flex', justifyContent: 'center' }}>
        <CircularProgress />
      </Container>
    );
  }

  return (
    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4">
          Dashboard - {user?.name}
        </Typography>
        <Button 
          variant="outlined" 
          startIcon={<Refresh />}
          onClick={fetchDashboardData}
          disabled={loading}
        >
          Refresh
        </Button>
      </Box>
      
      <Grid container spacing={3} mb={4}>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Total Accounts"
            value={stats?.totalAccounts || 0}
            icon={<AccountBalance fontSize="large" />}
            color="primary.main"
          />
        </Grid>
        
        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Transactions"
            value={stats?.totalTransactions || 0}
            subtitle={`${stats?.draftTransactions || 0} drafts`}
            icon={<Receipt fontSize="large" />}
            color="secondary.main"
            alert={stats?.draftTransactions > 5 ? "Review drafts" : null}
          />
        </Grid>
        
        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Invoices"
            value={stats?.totalInvoices || 0}
            subtitle={`${stats?.pendingInvoices || 0} pending`}
            icon={<TrendingUp fontSize="large" />}
            color="success.main"
            alert={stats?.overdueInvoices > 0 ? `${stats.overdueInvoices} overdue` : null}
          />
        </Grid>
        
        <Grid item xs={12} sm={6} md={3}>
          <StatCard
            title="Revenue"
            value={stats?.totalRevenue || 0}
            icon={<TrendingUp fontSize="large" />}
            color="info.main"
          />
        </Grid>
      </Grid>

      <Typography variant="h5" gutterBottom>
        Quick Actions
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12} sm={6} md={3}>
          <Button variant="contained" fullWidth startIcon={<Receipt />} href="/transactions">
            New Transaction
          </Button>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Button variant="contained" fullWidth startIcon={<TrendingUp />} href="/invoices">
            Create Invoice
          </Button>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Button variant="outlined" fullWidth startIcon={<AccountBalance />} href="/accounts">
            Manage Accounts
          </Button>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Button variant="outlined" fullWidth startIcon={<Warning />} href="/reports">
            View Reports
          </Button>
        </Grid>
      </Grid>
    </Container>
  );
};

export default Dashboard;