// frontend/src/components/Transactions.js
import React, { useState, useEffect } from 'react';
import {
  Container, Typography, Button, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Paper, Dialog, DialogTitle,
  DialogContent, DialogActions, TextField, Box, Chip, Alert, Grid,
  IconButton, Tooltip, Divider
} from '@mui/material';
import { Add, PostAdd, Delete, Edit } from '@mui/icons-material';
import { useAuth } from '../contexts/AuthContext';
import { useNotification } from '../contexts/NotificationContext';
import ApiService from '../services/api';
import Loading from './Loading';
import { useFormValidation, validationRules } from '../utils/formValidation';

const Transactions = () => {
  const [transactions, setTransactions] = useState([]);
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const { user } = useAuth();
  const { showSuccess, showError } = useNotification();

  const {
    values, errors, touched, handleChange, handleBlur, 
    validateAll, resetForm, setValues
  } = useFormValidation(
    {
      entry_number: '',
      entry_date: new Date().toISOString().split('T')[0],
      description: '',
      lines: [
        { account_id: '', description: '', debit_amount: 0, credit_amount: 0 },
        { account_id: '', description: '', debit_amount: 0, credit_amount: 0 }
      ]
    },
    {
      entry_number: [validationRules.required],
      entry_date: [validationRules.required],
      description: [validationRules.required]
    }
  );

  useEffect(() => {
    if (user?.company_id) {
      fetchData();
    }
  }, [user]);

  const fetchData = async () => {
    try {
      const [transRes, accRes] = await Promise.all([
        ApiService.getTransactions(user.company_id),
        ApiService.getAccounts(user.company_id)
      ]);
      setTransactions(transRes.data.data || []);
      setAccounts(accRes.data.data || []);
    } catch (error) {
      showError(`Failed to load data: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const validateTransaction = () => {
    if (!validateAll()) return false;
    
    const validLines = values.lines.filter(line => line.account_id);
    if (validLines.length < 2) {
      showError('At least two journal lines are required');
      return false;
    }
    
    const totalDebits = validLines.reduce((sum, line) => sum + (parseFloat(line.debit_amount) || 0), 0);
    const totalCredits = validLines.reduce((sum, line) => sum + (parseFloat(line.credit_amount) || 0), 0);
    
    if (Math.abs(totalDebits - totalCredits) > 0.01) {
      showError('Debits must equal credits');
      return false;
    }
    
    if (totalDebits === 0) {
      showError('Transaction must have non-zero amounts');
      return false;
    }
    
    return true;
  };

  const handleSubmit = async () => {
    if (!validateTransaction()) return;
    
    try {
      setSubmitting(true);
      
      const transactionData = {
        ...values,
        company_id: user.company_id,
        created_by: user.id,
        lines: values.lines.filter(line => line.account_id)
      };

      await ApiService.createTransaction(transactionData);
      showSuccess('Transaction created successfully');
      fetchData();
      handleClose();
    } catch (error) {
      if (error.code === 'DUPLICATE_ENTRY') {
        showError('Entry number already exists');
      } else {
        showError(`Error: ${error.message}`);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handlePostTransaction = async (id) => {
    if (!window.confirm('Are you sure you want to post this transaction? This cannot be undone.')) {
      return;
    }

    try {
      await ApiService.postTransaction(id);
      showSuccess('Transaction posted successfully');
      fetchData();
    } catch (error) {
      showError(`Error posting transaction: ${error.message}`);
    }
  };

  const handleClose = () => {
    setOpen(false);
    resetForm();
  };

  const updateLine = (index, field, value) => {
    const newLines = [...values.lines];
    newLines[index][field] = value;
    
    // Clear the opposite field when one is entered
    if (field === 'debit_amount' && value > 0) {
      newLines[index].credit_amount = 0;
    } else if (field === 'credit_amount' && value > 0) {
      newLines[index].debit_amount = 0;
    }
    
    handleChange('lines', newLines);
  };

  const addLine = () => {
    handleChange('lines', [...values.lines, { account_id: '', description: '', debit_amount: 0, credit_amount: 0 }]);
  };

  const removeLine = (index) => {
    if (values.lines.length > 2) {
      const newLines = values.lines.filter((_, i) => i !== index);
      handleChange('lines', newLines);
    }
  };

  const generateEntryNumber = () => {
    const date = new Date().toISOString().slice(0, 10).replace(/-/g, '');
    const random = Math.floor(Math.random() * 1000).toString().padStart(3, '0');
    return `JE${date}${random}`;
  };

  if (loading) return <Loading type="skeleton" rows={8} />;

  const totalDebits = values.lines.reduce((sum, line) => sum + (parseFloat(line.debit_amount) || 0), 0);
  const totalCredits = values.lines.reduce((sum, line) => sum + (parseFloat(line.credit_amount) || 0), 0);
  const isBalanced = Math.abs(totalDebits - totalCredits) < 0.01;

  return (
    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4">Journal Entries</Typography>
        <Button variant="contained" startIcon={<Add />} onClick={() => setOpen(true)}>
          New Transaction
        </Button>
      </Box>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Entry Number</TableCell>
              <TableCell>Date</TableCell>
              <TableCell>Description</TableCell>
              <TableCell align="right">Amount</TableCell>
              <TableCell>Status</TableCell>
              <TableCell align="center">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {transactions.length > 0 ? (
              transactions.map((transaction) => (
                <TableRow key={transaction.id} hover>
                  <TableCell sx={{ fontFamily: 'monospace', fontWeight: 'bold' }}>
                    {transaction.entry_number}
                  </TableCell>
                  <TableCell>
                    {new Date(transaction.entry_date).toLocaleDateString('id-ID')}
                  </TableCell>
                  <TableCell>{transaction.description}</TableCell>
                  <TableCell align="right">
                    Rp {transaction.total_amount?.toLocaleString('id-ID')}
                  </TableCell>
                  <TableCell>
                    <Chip 
                      label={transaction.status}
                      color={transaction.status === 'posted' ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell align="center">
                    {transaction.status === 'draft' && (
                      <Tooltip title="Post Transaction">
                        <IconButton
                          size="small"
                          onClick={() => handlePostTransaction(transaction.id)}
                          color="primary"
                        >
                          <PostAdd />
                        </IconButton>
                      </Tooltip>
                    )}
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={6} align="center" sx={{ py: 4 }}>
                  <Typography variant="body1" color="textSecondary">
                    No transactions found. Create your first journal entry to get started.
                  </Typography>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Create Dialog */}
      <Dialog open={open} onClose={handleClose} maxWidth="lg" fullWidth>
        <DialogTitle>New Journal Entry</DialogTitle>
        <DialogContent>
          <Grid container spacing={2} sx={{ mb: 3 }}>
            <Grid item xs={6}>
              <TextField
                label="Entry Number"
                fullWidth
                value={values.entry_number}
                onChange={(e) => handleChange('entry_number', e.target.value)}
                onBlur={() => handleBlur('entry_number')}
                error={touched.entry_number && !!errors.entry_number}
                helperText={touched.entry_number && errors.entry_number}
                disabled={submitting}
                InputProps={{
                  endAdornment: (
                    <Button size="small" onClick={() => handleChange('entry_number', generateEntryNumber())}>
                      Generate
                    </Button>
                  )
                }}
              />
            </Grid>
            <Grid item xs={6}>
              <TextField
                label="Entry Date"
                type="date"
                fullWidth
                InputLabelProps={{ shrink: true }}
                value={values.entry_date}
                onChange={(e) => handleChange('entry_date', e.target.value)}
                disabled={submitting}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                label="Description"
                fullWidth
                multiline
                rows={2}
                value={values.description}
                onChange={(e) => handleChange('description', e.target.value)}
                onBlur={() => handleBlur('description')}
                error={touched.description && !!errors.description}
                helperText={touched.description && errors.description}
                disabled={submitting}
              />
            </Grid>
          </Grid>

          {!isBalanced && totalDebits > 0 && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              Debits (Rp {totalDebits.toLocaleString('id-ID')}) must equal Credits (Rp {totalCredits.toLocaleString('id-ID')})
            </Alert>
          )}

          <Typography variant="h6" gutterBottom>
            Journal Lines
          </Typography>

          {values.lines.map((line, index) => (
            <Paper key={index} sx={{ p: 2, mb: 2, bgcolor: 'grey.50' }}>
              <Grid container spacing={2} alignItems="center">
                <Grid item xs={3}>
                  <TextField
                    select
                    label="Account"
                    fullWidth
                    value={line.account_id}
                    onChange={(e) => updateLine(index, 'account_id', e.target.value)}
                    SelectProps={{ native: true }}
                    size="small"
                  >
                    <option value="">Select Account</option>
                    {accounts.map((account) => (
                      <option key={account.id} value={account.id}>
                        {account.account_code} - {account.account_name}
                      </option>
                    ))}
                  </TextField>
                </Grid>
                <Grid item xs={3}>
                  <TextField
                    label="Description"
                    fullWidth
                    value={line.description}
                    onChange={(e) => updateLine(index, 'description', e.target.value)}
                    size="small"
                  />
                </Grid>
                <Grid item xs={2}>
                  <TextField
                    label="Debit"
                    type="number"
                    fullWidth
                    value={line.debit_amount}
                    onChange={(e) => updateLine(index, 'debit_amount', parseFloat(e.target.value) || 0)}
                    size="small"
                    inputProps={{ min: 0, step: 0.01 }}
                  />
                </Grid>
                <Grid item xs={2}>
                  <TextField
                    label="Credit"
                    type="number"
                    fullWidth
                    value={line.credit_amount}
                    onChange={(e) => updateLine(index, 'credit_amount', parseFloat(e.target.value) || 0)}
                    size="small"
                    inputProps={{ min: 0, step: 0.01 }}
                  />
                </Grid>
                <Grid item xs={2}>
                  <Box display="flex" gap={1}>
                    <Tooltip title="Remove Line">
                      <IconButton
                        size="small"
                        onClick={() => removeLine(index)}
                        disabled={values.lines.length <= 2}
                        color="error"
                      >
                        <Delete />
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Grid>
              </Grid>
            </Paper>
          ))}

          <Button onClick={addLine} variant="outlined" sx={{ mb: 2 }}>
            Add Line
          </Button>

          <Divider sx={{ my: 2 }} />
          
          <Grid container spacing={2}>
            <Grid item xs={6}>
              <TextField 
                label="Total Debits" 
                fullWidth 
                value={`Rp ${totalDebits.toLocaleString('id-ID')}`} 
                InputProps={{ readOnly: true }}
                color={isBalanced ? 'success' : 'warning'}
              />
            </Grid>
            <Grid item xs={6}>
              <TextField 
                label="Total Credits" 
                fullWidth 
                value={`Rp ${totalCredits.toLocaleString('id-ID')}`} 
                InputProps={{ readOnly: true }}
                color={isBalanced ? 'success' : 'warning'}
              />
            </Grid>
          </Grid>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} disabled={submitting}>
            Cancel
          </Button>
          <Button 
            onClick={handleSubmit} 
            variant="contained" 
            disabled={submitting || !isBalanced || totalDebits === 0}
          >
            {submitting ? 'Creating...' : 'Create Entry'}
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default Transactions;