import { useState, useCallback } from 'react';
import { useNotification } from '../contexts/NotificationContext';

export const useApi = () => {
  const [loading, setLoading] = useState(false);
  const { showError, showSuccess } = useNotification();

  const execute = useCallback(async (apiCall, successMessage) => {
    try {
      setLoading(true);
      const result = await apiCall();
      if (successMessage) showSuccess(successMessage);
      return result;
    } catch (error) {
      showError(error.response?.data?.error || 'An error occurred');
      throw error;
    } finally {
      setLoading(false);
    }
  }, [showError, showSuccess]);

  return { execute, loading };
};