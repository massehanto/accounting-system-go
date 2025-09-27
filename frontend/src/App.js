import React, { useState, useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { Snackbar, Alert } from '@mui/material';
import CssBaseline from '@mui/material/CssBaseline';
import ErrorBoundary from './components/ErrorBoundary';
import Loading from './components/Loading';
import Login from './components/Login';
import Dashboard from './components/Dashboard';
import ChartOfAccounts from './components/ChartOfAccounts';
import Transactions from './components/Transactions';
import Invoices from './components/Invoices';
import Reports from './components/Reports';
import Navigation from './components/Navigation';
import { AuthProvider, useAuth } from './contexts/AuthContext';
import { NotificationProvider, useNotification } from './contexts/NotificationContext';

const theme = createTheme({
  palette: {
    primary: { main: '#1976d2' },
    secondary: { main: '#dc004e' },
  },
  components: {
    MuiButton: {
      styleOverrides: {
        root: { textTransform: 'none' },
      },
    },
  },
});

function AppContent() {
  const { user, isAuthenticated, loading } = useAuth();
  const { notification, closeNotification } = useNotification();

  if (loading) {
    return <Loading message="Initializing application..." />;
  }

  return (
    <Router>
      {isAuthenticated && <Navigation />}
      <Routes>
        <Route path="/login" element={
          isAuthenticated ? <Navigate to="/dashboard" /> : <Login />
        } />
        <Route path="/dashboard" element={
          isAuthenticated ? <Dashboard /> : <Navigate to="/login" />
        } />
        <Route path="/accounts" element={
          isAuthenticated ? <ChartOfAccounts /> : <Navigate to="/login" />
        } />
        <Route path="/transactions" element={
          isAuthenticated ? <Transactions /> : <Navigate to="/login" />
        } />
        <Route path="/invoices" element={
          isAuthenticated ? <Invoices /> : <Navigate to="/login" />
        } />
        <Route path="/reports" element={
          isAuthenticated ? <Reports /> : <Navigate to="/login" />
        } />
        <Route path="/" element={<Navigate to="/dashboard" />} />
      </Routes>
      
      <Snackbar 
        open={!!notification} 
        autoHideDuration={6000} 
        onClose={closeNotification}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      >
        {notification && (
          <Alert onClose={closeNotification} severity={notification.type}>
            {notification.message}
          </Alert>
        )}
      </Snackbar>
    </Router>
  );
}

function App() {
  return (
    <ErrorBoundary>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <AuthProvider>
          <NotificationProvider>
            <AppContent />
          </NotificationProvider>
        </AuthProvider>
      </ThemeProvider>
    </ErrorBoundary>
  );
}

export default App;