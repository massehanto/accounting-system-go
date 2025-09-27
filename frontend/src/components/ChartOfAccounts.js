// frontend/src/components/ChartOfAccounts.js
import React, { useState, useEffect } from 'react';
import {
  Container, Typography, Button, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Paper, Dialog, DialogTitle,
  DialogContent, DialogActions, TextField, Box, Chip, Alert,
  IconButton, Menu, MenuItem, Tooltip, InputAdornment
} from '@mui/material';
import { 
  Add, Edit, Delete, MoreVert, Search, FilterList,
  AccountBalance, TrendingUp, TrendingDown
} from '@mui/icons-material';
import { useAuth } from '../contexts/AuthContext';
import { useNotification } from '../contexts/NotificationContext';
import ApiService from '../services/api';
import Loading from './Loading';
import { useFormValidation, validationRules } from '../utils/formValidation';

const ChartOfAccounts = () => {
  const [accounts, setAccounts] = useState([]);
  const [filteredAccounts, setFilteredAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editingAccount, setEditingAccount] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterType, setFilterType] = useState('All');
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedAccount, setSelectedAccount] = useState(null);
  
  const { user } = useAuth();
  const { showSuccess, showError } = useNotification();

  const accountTypes = ['All', 'Asset', 'Liability', 'Equity', 'Revenue', 'Expense'];

  const {
    values, errors, touched, handleChange, handleBlur, 
    validateAll, resetForm, setValues
  } = useFormValidation(
    {
      account_code: '',
      account_name: '',
      account_type: '',
      parent_id: null,
      is_active: true
    },
    {
      account_code: [validationRules.required, validationRules.accountCode],
      account_name: [validationRules.required],
      account_type: [validationRules.required]
    }
  );

  useEffect(() => {
    if (user?.company_id) {
      fetchAccounts();
    }
  }, [user]);

  useEffect(() => {
    filterAccounts();
  }, [accounts, searchTerm, filterType]);

  const fetchAccounts = async () => {
    try {
      setLoading(true);
      const response = await ApiService.getAccounts(user.company_id);
      const accountData = response.data?.data || [];
      setAccounts(accountData);
    } catch (error) {
      showError(`Failed to load accounts: ${error.message}`);
      setAccounts([]);
    } finally {
      setLoading(false);
    }
  };

  const filterAccounts = () => {
    let filtered = accounts;
    
    if (searchTerm) {
      filtered = filtered.filter(account => 
        account.account_code.toLowerCase().includes(searchTerm.toLowerCase()) ||
        account.account_name.toLowerCase().includes(searchTerm.toLowerCase())
      );
    }
    
    if (filterType !== 'All') {
      filtered = filtered.filter(account => account.account_type === filterType);
    }
    
    setFilteredAccounts(filtered);
  };

  const handleSubmit = async () => {
    if (!validateAll()) {
      showError('Please correct the form errors');
      return;
    }
    
    try {
      setSubmitting(true);
      
      const accountData = {
        ...values,
        company_id: user.company_id,
        parent_id: values.parent_id || null
      };

      if (editingAccount) {
        await ApiService.updateAccount(editingAccount.id, accountData);
        showSuccess('Account updated successfully');
      } else {
        await ApiService.createAccount(accountData);
        showSuccess('Account created successfully');
      }
      
      await fetchAccounts();
      handleClose();
    } catch (error) {
      if (error.code === 'DUPLICATE_CODE') {
        showError('Account code already exists');
      } else if (error.code === 'VALIDATION_ERROR') {
        showError('Please check the form for errors');
      } else {
        showError(`Error: ${error.message}`);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleEdit = (account) => {
    setEditingAccount(account);
    setValues({
      account_code: account.account_code,
      account_name: account.account_name,
      account_type: account.account_type,
      parent_id: account.parent_id,
      is_active: account.is_active
    });
    setOpen(true);
    handleMenuClose();
  };

  const handleDelete = async (account) => {
    if (window.confirm(`Are you sure you want to delete account "${account.account_name}"?`)) {
      try {
        await ApiService.deleteAccount(account.id);
        showSuccess('Account deleted successfully');
        fetchAccounts();
      } catch (error) {
        showError(`Error deleting account: ${error.message}`);
      }
    }
    handleMenuClose();
  };

  const handleClose = () => {
    setOpen(false);
    setEditingAccount(null);
    resetForm();
  };

  const handleMenuClick = (event, account) => {
    setAnchorEl(event.currentTarget);
    setSelectedAccount(account);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
    setSelectedAccount(null);
  };

  const getAccountIcon = (type) => {
    switch (type) {
      case 'Asset': return <TrendingUp color="success" />;
      case 'Liability': return <TrendingDown color="error" />;
      case 'Equity': return <AccountBalance color="info" />;
      case 'Revenue': return <TrendingUp color="primary" />;
      case 'Expense': return <TrendingDown color="warning" />;
      default: return <AccountBalance />;
    }
  };

  const getAccountTypeColor = (type) => {
    switch (type) {
      case 'Asset': return 'success';
      case 'Liability': return 'error';
      case 'Equity': return 'info';
      case 'Revenue': return 'primary';
      case 'Expense': return 'warning';
      default: return 'default';
    }
  };

  if (loading) return <Loading type="skeleton" rows={10} />;

  return (
    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4" sx={{ display: 'flex', alignItems: 'center' }}>
          <AccountBalance sx={{ mr: 1 }} />
          Chart of Accounts
        </Typography>
        <Button 
          variant="contained" 
          startIcon={<Add />} 
          onClick={() => setOpen(true)}
          size="large"
        >
          Add Account
        </Button>
      </Box>

      {/* Search and Filter Bar */}
      <Paper sx={{ p: 2, mb: 3 }}>
        <Box display="flex" gap={2} alignItems="center">
          <TextField
            placeholder="Search accounts..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            size="small"
            sx={{ flexGrow: 1 }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <Search />
                </InputAdornment>
              ),
            }}
          />
          <TextField
            select
            label="Filter by Type"
            value={filterType}
            onChange={(e) => setFilterType(e.target.value)}
            size="small"
            sx={{ minWidth: 150 }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <FilterList />
                </InputAdornment>
              ),
            }}
          >
            {accountTypes.map((type) => (
              <MenuItem key={type} value={type}>{type}</MenuItem>
            ))}
          </TextField>
        </Box>
      </Paper>

      {/* Accounts Table */}
      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell><strong>Code</strong></TableCell>
              <TableCell><strong>Name</strong></TableCell>
              <TableCell><strong>Type</strong></TableCell>
              <TableCell><strong>Status</strong></TableCell>
              <TableCell align="right"><strong>Actions</strong></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {filteredAccounts.length > 0 ? (
              filteredAccounts.map((account) => (
                <TableRow key={account.id} hover>
                  <TableCell sx={{ fontFamily: 'monospace', fontWeight: 'bold' }}>
                    {account.account_code}
                  </TableCell>
                  <TableCell>
                    <Box display="flex" alignItems="center">
                      {getAccountIcon(account.account_type)}
                      <Box ml={1}>
                        {account.account_name}
                        {account.parent_id && (
                          <Typography variant="caption" display="block" color="textSecondary">
                            Child account
                          </Typography>
                        )}
                      </Box>
                    </Box>
                  </TableCell>
                  <TableCell>
                    <Chip 
                      label={account.account_type}
                      color={getAccountTypeColor(account.account_type)}
                      size="small"
                      variant="outlined"
                    />
                  </TableCell>
                  <TableCell>
                    <Chip 
                      label={account.is_active ? 'Active' : 'Inactive'}
                      color={account.is_active ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell align="right">
                    <Tooltip title="More actions">
                      <IconButton
                        size="small"
                        onClick={(e) => handleMenuClick(e, account)}
                      >
                        <MoreVert />
                      </IconButton>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                  <Typography variant="body1" color="textSecondary">
                    {searchTerm || filterType !== 'All' 
                      ? 'No accounts match your search criteria' 
                      : 'No accounts found. Create your first account to get started.'}
                  </Typography>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Actions Menu */}
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={() => handleEdit(selectedAccount)}>
          <Edit sx={{ mr: 1 }} /> Edit
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedAccount)}>
          <Delete sx={{ mr: 1 }} /> Delete
        </MenuItem>
      </Menu>

      {/* Create/Edit Dialog */}
      <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
        <DialogTitle>
          {editingAccount ? 'Edit Account' : 'Add New Account'}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ pt: 1 }}>
            <TextField
              autoFocus
              margin="normal"
              label="Account Code"
              fullWidth
              value={values.account_code}
              onChange={(e) => handleChange('account_code', e.target.value)}
              onBlur={() => handleBlur('account_code')}
              error={touched.account_code && !!errors.account_code}
              helperText={touched.account_code && errors.account_code || 'Must be 4 digits (e.g., 1000)'}
              disabled={submitting || !!editingAccount}
              placeholder="1000"
            />
            
            <TextField
              margin="normal"
              label="Account Name"
              fullWidth
              value={values.account_name}
              onChange={(e) => handleChange('account_name', e.target.value)}
              onBlur={() => handleBlur('account_name')}
              error={touched.account_name && !!errors.account_name}
              helperText={touched.account_name && errors.account_name}
              disabled={submitting}
              placeholder="Enter account name"
            />
            
            <TextField
              select
              margin="normal"
              label="Account Type"
              fullWidth
              value={values.account_type}
              onChange={(e) => handleChange('account_type', e.target.value)}
              onBlur={() => handleBlur('account_type')}
              error={touched.account_type && !!errors.account_type}
              helperText={touched.account_type && errors.account_type}
              disabled={submitting}
            >
              {accountTypes.slice(1).map((type) => (
                <MenuItem key={type} value={type}>
                  <Box display="flex" alignItems="center">
                    {getAccountIcon(type)}
                    <Box ml={1}>{type}</Box>
                  </Box>
                </MenuItem>
              ))}
            </TextField>

            <TextField
              select
              margin="normal"
              label="Parent Account (Optional)"
              fullWidth
              value={values.parent_id || ''}
              onChange={(e) => handleChange('parent_id', e.target.value || null)}
              disabled={submitting}
              helperText="Select a parent account to create a sub-account"
            >
              <MenuItem value="">None (Top Level)</MenuItem>
              {accounts
                .filter(acc => acc.account_type === values.account_type && acc.id !== editingAccount?.id)
                .map((account) => (
                  <MenuItem key={account.id} value={account.id}>
                    {account.account_code} - {account.account_name}
                  </MenuItem>
                ))}
            </TextField>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} disabled={submitting}>
            Cancel
          </Button>
          <Button 
            onClick={handleSubmit} 
            variant="contained" 
            disabled={submitting}
          >
            {submitting ? 'Saving...' : (editingAccount ? 'Update' : 'Create')}
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default ChartOfAccounts;