// frontend/src/components/Dashboard.js - SIMPLIFIED VERSION
import React, { useState, useEffect } from 'react';
import {
  Container, Grid, Paper, Typography, Box, Card, CardContent
} from '@mui/material';
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
      
      const [accountsRes, transactionsRes] = await Promise.all([
        ApiService.getAccounts(user.company_id),
        ApiService.getTransactions(user.company_id)
      ]);

      const accounts = accountsRes.data?.data || [];
      const transactions = transactionsRes.data?.data || [];

      setStats({
        totalAccounts: accounts.length,
        totalTransactions: transactions.length,
        draftTransactions: transactions.filter(tx => tx.status === 'draft').length,
      });
    } catch (error) {
      showError('Failed to load dashboard data');
    } finally {
      setLoading(false);
    }
  };

  if (loading) return <div>Loading...</div>;

  return (
    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
      <Typography variant="h4" gutterBottom>
        Dashboard - {user?.name}
      </Typography>
      
      <Grid container spacing={3}>
        <Grid item xs={12} sm={6} md={4}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Total Accounts
              </Typography>
              <Typography variant="h4">
                {stats?.totalAccounts || 0}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        
        <Grid item xs={12} sm={6} md={4}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Transactions
              </Typography>
              <Typography variant="h4">
                {stats?.totalTransactions || 0}
              </Typography>
              <Typography variant="body2">
                {stats?.draftTransactions || 0} drafts
              </Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Container>
  );
};

export default Dashboard;