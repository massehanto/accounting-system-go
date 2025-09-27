// frontend/src/components/Login.js - SIMPLIFIED VERSION
import React, { useState } from 'react';
import {
  Container, Paper, TextField, Button, Typography, Box, Alert
} from '@mui/material';
import { useAuth } from '../contexts/AuthContext';
import { useFormValidation, validationRules } from '../utils/formValidation';

const Login = () => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const { login } = useAuth();

  const {
    values, errors, touched, handleChange, handleBlur, validateAll
  } = useFormValidation(
    { email: '', password: '' },
    {
      email: [validationRules.required, validationRules.email],
      password: [validationRules.required]
    }
  );

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateAll()) return;

    setLoading(true);
    setError('');
    
    try {
      await login(values);
    } catch (err) {
      setError(err.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Container component="main" maxWidth="xs">
      <Box sx={{ marginTop: 8, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        <Paper elevation={3} sx={{ padding: 4, width: '100%' }}>
          <Typography variant="h4" component="h1" align="center" gutterBottom>
            Sistem Akuntansi
          </Typography>
          
          {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
          
          <Box component="form" onSubmit={handleSubmit}>
            <TextField
              required
              fullWidth
              label="Email"
              type="email"
              value={values.email}
              onChange={(e) => handleChange('email', e.target.value)}
              onBlur={() => handleBlur('email')}
              error={touched.email && !!errors.email}
              helperText={touched.email && errors.email}
              disabled={loading}
              sx={{ mb: 2 }}
            />
            
            <TextField
              required
              fullWidth
              label="Password"
              type="password"
              value={values.password}
              onChange={(e) => handleChange('password', e.target.value)}
              onBlur={() => handleBlur('password')}
              error={touched.password && !!errors.password}
              helperText={touched.password && errors.password}
              disabled={loading}
              sx={{ mb: 3 }}
            />
            
            <Button
              type="submit"
              fullWidth
              variant="contained"
              disabled={loading}
            >
              {loading ? 'Signing In...' : 'Sign In'}
            </Button>
          </Box>
        </Paper>
      </Box>
    </Container>
  );
};

export default Login;