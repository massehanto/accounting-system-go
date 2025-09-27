// frontend/src/components/Reports.js (Enhanced)
import React, { useState, useEffect } from 'react';
import {
  Container, Typography, Grid, Paper, Table, TableBody,
  TableCell, TableContainer, TableHead, TableRow, Box,
  FormControl, InputLabel, Select, MenuItem, Button, Card, CardContent, Alert
} from '@mui/material';
import { Print, GetApp, Assessment } from '@mui/icons-material';
import { useAuth } from '../contexts/AuthContext';
import { useNotification } from '../contexts/NotificationContext';
import ApiService from '../services/api';
import Loading from './Loading';

const Reports = () => {
  const [reportType, setReportType] = useState('balance_sheet');
  const [reportData, setReportData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const { user } = useAuth();
  const { showError } = useNotification();

  const generateReport = async () => {
    if (!reportType) return;
    
    try {
      setLoading(true);
      setError(null);
      
      const response = await ApiService.api.post('/reports/generate', {
        report_type: reportType,
        start_date: '2024-01-01',
        end_date: '2024-12-31'
      });
      
      setReportData(response.data);
    } catch (error) {
      setError(error);
      showError(`Failed to generate report: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    generateReport();
  }, [reportType]);

  const handlePrint = () => window.print();
  
  const handleExport = () => {
    const dataStr = JSON.stringify(reportData, null, 2);
    const dataBlob = new Blob([dataStr], { type: 'application/json' });
    const url = URL.createObjectURL(dataBlob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${reportType}_${new Date().toISOString().split('T')[0]}.json`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const renderBalanceSheet = () => (
    <Grid container spacing={3}>
      <Grid item xs={6}>
        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom color="primary">
              ASSETS
            </Typography>
            <Table size="small">
              <TableBody>
                <TableRow>
                  <TableCell>Cash and Equivalents</TableCell>
                  <TableCell align="right">Rp 150,000,000</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>Accounts Receivable</TableCell>
                  <TableCell align="right">Rp 75,000,000</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>Inventory</TableCell>
                  <TableCell align="right">Rp 50,000,000</TableCell>
                </TableRow>
                <TableRow sx={{ bgcolor: 'grey.100' }}>
                  <TableCell><strong>Total Assets</strong></TableCell>
                  <TableCell align="right"><strong>Rp 275,000,000</strong></TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </Grid>
      
      <Grid item xs={6}>
        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom color="secondary">
              LIABILITIES & EQUITY
            </Typography>
            <Table size="small">
              <TableBody>
                <TableRow>
                  <TableCell>Accounts Payable</TableCell>
                  <TableCell align="right">Rp 25,000,000</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>Share Capital</TableCell>
                  <TableCell align="right">Rp 200,000,000</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>Retained Earnings</TableCell>
                  <TableCell align="right">Rp 50,000,000</TableCell>
                </TableRow>
                <TableRow sx={{ bgcolor: 'grey.100' }}>
                  <TableCell><strong>Total Liab. & Equity</strong></TableCell>
                  <TableCell align="right"><strong>Rp 275,000,000</strong></TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </Grid>
    </Grid>
  );

  if (loading) return <Loading message="Generating report..." />;

  return (
    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4" sx={{ display: 'flex', alignItems: 'center' }}>
          <Assessment sx={{ mr: 1 }} />
          Financial Reports
        </Typography>
        <Box className="no-print">
          <Button startIcon={<Print />} onClick={handlePrint} sx={{ mr: 1 }}>
            Print
          </Button>
          <Button startIcon={<GetApp />} onClick={handleExport}>
            Export
          </Button>
        </Box>
      </Box>

      <Box mb={3} className="no-print">
        <FormControl variant="outlined" sx={{ minWidth: 200 }}>
          <InputLabel>Report Type</InputLabel>
          <Select
            value={reportType}
            label="Report Type"
            onChange={(e) => setReportType(e.target.value)}
          >
            <MenuItem value="balance_sheet">Balance Sheet</MenuItem>
            <MenuItem value="income_statement">Income Statement</MenuItem>
            <MenuItem value="trial_balance">Trial Balance</MenuItem>
            <MenuItem value="cash_flow">Cash Flow Statement</MenuItem>
          </Select>
        </FormControl>
        <Button 
          variant="outlined" 
          onClick={generateReport} 
          sx={{ ml: 2 }}
          disabled={loading}
        >
          Generate Report
        </Button>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          Error generating report: {error.message}
        </Alert>
      )}

      {reportData && reportType === 'balance_sheet' && renderBalanceSheet()}
    </Container>
  );
};

export default Reports;