// frontend/src/components/Invoices.js (Enhanced)
import React, { useState, useEffect } from 'react';
import {
  Container, Typography, Button, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Paper, Dialog, DialogTitle,
  DialogContent, DialogActions, TextField, Box, Chip, Grid, Divider
} from '@mui/material';
import { Add, Send, Print } from '@mui/icons-material';
import { useAuth } from '../contexts/AuthContext';
import { useNotification } from '../contexts/NotificationContext';
import ApiService from '../services/api';
import Loading from './Loading';
import { useFormValidation, validationRules } from '../utils/formValidation';

const Invoices = () => {
  const [invoices, setInvoices] = useState([]);
  const [customers, setCustomers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const { user } = useAuth();
  const { showSuccess, showError } = useNotification();

  const {
    values, errors, touched, handleChange, handleBlur, 
    validateAll, resetForm
  } = useFormValidation(
    {
      customer_id: '',
      invoice_number: '',
      invoice_date: new Date().toISOString().split('T')[0],
      due_date: '',
      lines: [{ product_name: '', quantity: 1, unit_price: 0, line_total: 0 }]
    },
    {
      customer_id: [validationRules.required],
      invoice_number: [validationRules.required],
      invoice_date: [validationRules.required],
      due_date: [validationRules.required]
    }
  );

  useEffect(() => {
    fetchData();
  }, [user]);

  const fetchData = async () => {
    try {
      const [invoicesRes, customersRes] = await Promise.all([
        ApiService.getInvoices(user.company_id),
        ApiService.getCustomers(user.company_id)
      ]);
      setInvoices(invoicesRes.data || []);
      setCustomers(customersRes.data || []);
    } catch (error) {
      showError(`Failed to load data: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const calculateTotals = (lines) => {
    const subtotal = lines.reduce((sum, line) => sum + line.line_total, 0);
    const tax = subtotal * 0.11;
    return { subtotal, tax, total: subtotal + tax };
  };

  const updateLine = (index, field, value) => {
    const newLines = [...values.lines];
    newLines[index][field] = value;
    
    if (field === 'quantity' || field === 'unit_price') {
      newLines[index].line_total = newLines[index].quantity * newLines[index].unit_price;
    }
    
    handleChange('lines', newLines);
  };

  const addLine = () => {
    handleChange('lines', [...values.lines, { product_name: '', quantity: 1, unit_price: 0, line_total: 0 }]);
  };

  const handleSubmit = async () => {
    if (!validateAll()) return;
    
    try {
      setSubmitting(true);
      const totals = calculateTotals(values.lines.filter(line => line.product_name));
      
      await ApiService.createInvoice({
        ...values,
        company_id: user.company_id,
        subtotal: totals.subtotal,
        tax_amount: totals.tax,
        total_amount: totals.total,
        lines: values.lines.filter(line => line.product_name)
      });
      
      showSuccess('Invoice created successfully');
      fetchData();
      handleClose();
    } catch (error) {
      showError(`Error: ${error.message}`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleClose = () => {
    setOpen(false);
    resetForm();
  };

  if (loading) return <Loading type="skeleton" rows={8} />;

  const totals = calculateTotals(values.lines || []);

  return (
    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4">Invoices</Typography>
        <Button variant="contained" startIcon={<Add />} onClick={() => setOpen(true)}>
          New Invoice
        </Button>
      </Box>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Invoice Number</TableCell>
              <TableCell>Customer</TableCell>
              <TableCell>Date</TableCell>
              <TableCell align="right">Amount</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {invoices.map((invoice) => (
              <TableRow key={invoice.id} hover>
                <TableCell sx={{ fontFamily: 'monospace' }}>
                  {invoice.invoice_number}
                </TableCell>
                <TableCell>{invoice.customer?.name}</TableCell>
                <TableCell>{new Date(invoice.invoice_date).toLocaleDateString('id-ID')}</TableCell>
                <TableCell align="right">Rp {invoice.total_amount?.toLocaleString('id-ID')}</TableCell>
                <TableCell>
                  <Chip 
                    label={invoice.status}
                    color={invoice.status === 'paid' ? 'success' : 'warning'}
                    size="small"
                  />
                </TableCell>
                <TableCell>
                  <Button size="small" startIcon={<Send />}>Send</Button>
                  <Button size="small" startIcon={<Print />}>Print</Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={open} onClose={handleClose} maxWidth="lg" fullWidth>
        <DialogTitle>New Invoice</DialogTitle>
        <DialogContent>
          <Grid container spacing={2} sx={{ mb: 3 }}>
            <Grid item xs={6}>
              <TextField
                select
                label="Customer"
                fullWidth
                value={values.customer_id}
                onChange={(e) => handleChange('customer_id', e.target.value)}
                error={touched.customer_id && !!errors.customer_id}
                helperText={touched.customer_id && errors.customer_id}
                SelectProps={{ native: true }}
              >
                <option value="">Select Customer</option>
                {customers.map((customer) => (
                  <option key={customer.id} value={customer.id}>{customer.name}</option>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={6}>
              <TextField
                label="Invoice Number"
                fullWidth
                value={values.invoice_number}
                onChange={(e) => handleChange('invoice_number', e.target.value)}
                error={touched.invoice_number && !!errors.invoice_number}
                helperText={touched.invoice_number && errors.invoice_number}
              />
            </Grid>
          </Grid>

          <Typography variant="h6" gutterBottom>Invoice Lines</Typography>
          {values.lines?.map((line, index) => (
            <Paper key={index} sx={{ p: 2, mb: 2, bgcolor: 'grey.50' }}>
              <Grid container spacing={2}>
                <Grid item xs={4}>
                  <TextField
                    label="Product/Service"
                    fullWidth
                    value={line.product_name}
                    onChange={(e) => updateLine(index, 'product_name', e.target.value)}
                  />
                </Grid>
                <Grid item xs={2}>
                  <TextField
                    label="Quantity"
                    type="number"
                    fullWidth
                    value={line.quantity}
                    onChange={(e) => updateLine(index, 'quantity', parseFloat(e.target.value) || 0)}
                  />
                </Grid>
                <Grid item xs={3}>
                  <TextField
                    label="Unit Price"
                    type="number"
                    fullWidth
                    value={line.unit_price}
                    onChange={(e) => updateLine(index, 'unit_price', parseFloat(e.target.value) || 0)}
                  />
                </Grid>
                <Grid item xs={3}>
                  <TextField
                    label="Line Total"
                    fullWidth
                    value={line.line_total.toLocaleString('id-ID')}
                    InputProps={{ readOnly: true }}
                  />
                </Grid>
              </Grid>
            </Paper>
          ))}

          <Button onClick={addLine} variant="outlined" sx={{ mb: 2 }}>Add Line</Button>

          <Divider sx={{ my: 2 }} />
          
          <Grid container spacing={2} justifyContent="flex-end">
            <Grid item xs={4}>
              <TextField label="Subtotal" fullWidth value={`Rp ${totals.subtotal.toLocaleString('id-ID')}`} InputProps={{ readOnly: true }} />
            </Grid>
            <Grid item xs={4}>
              <TextField label="Tax (11%)" fullWidth value={`Rp ${totals.tax.toLocaleString('id-ID')}`} InputProps={{ readOnly: true }} />
            </Grid>
            <Grid item xs={4}>
              <TextField label="Total" fullWidth value={`Rp ${totals.total.toLocaleString('id-ID')}`} InputProps={{ readOnly: true }} />
            </Grid>
          </Grid>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} disabled={submitting}>Cancel</Button>
          <Button onClick={handleSubmit} variant="contained" disabled={submitting}>
            {submitting ? 'Creating...' : 'Create'}
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default Invoices;