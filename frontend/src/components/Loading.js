// frontend/src/components/Loading.js (Enhanced)
import React from 'react';
import { Box, CircularProgress, Typography, Skeleton } from '@mui/material';

const Loading = ({ 
  message = 'Loading...', 
  type = 'spinner',
  rows = 5,
  variant = 'text' 
}) => {
  if (type === 'skeleton') {
    return (
      <Box>
        {[...Array(rows)].map((_, index) => (
          <Skeleton 
            key={index} 
            variant={variant} 
            height={variant === 'text' ? 20 : 40}
            sx={{ mb: 1 }}
          />
        ))}
      </Box>
    );
  }

  return (
    <Box
      display="flex"
      flexDirection="column"
      justifyContent="center"
      alignItems="center"
      minHeight="200px"
      role="status"
      aria-live="polite"
    >
      <CircularProgress size={40} />
      <Typography 
        variant="body2" 
        sx={{ mt: 2 }}
        color="textSecondary"
      >
        {message}
      </Typography>
    </Box>
  );
};

export default Loading;