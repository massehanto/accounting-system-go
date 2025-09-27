// frontend/src/utils/formValidation.js - SIMPLIFIED VERSION
import { useState, useCallback } from 'react';

export const useFormValidation = (initialValues, validationRules) => {
    const [values, setValues] = useState(initialValues);
    const [errors, setErrors] = useState({});
    const [touched, setTouched] = useState({});

    const validateField = useCallback((name, value) => {
        const rules = validationRules[name] || [];
        for (const rule of rules) {
            const error = rule(value);
            if (error) return error;
        }
        return '';
    }, [validationRules]);

    const validateAll = useCallback(() => {
        const newErrors = {};
        let hasErrors = false;
        
        Object.keys(validationRules).forEach(field => {
            const error = validateField(field, values[field]);
            if (error) {
                newErrors[field] = error;
                hasErrors = true;
            }
        });
        
        setErrors(newErrors);
        setTouched(Object.keys(validationRules).reduce((acc, key) => ({ ...acc, [key]: true }), {}));
        
        return !hasErrors;
    }, [validationRules, validateField, values]);

    const handleChange = useCallback((name, value) => {
        setValues(prev => ({ ...prev, [name]: value }));
        
        if (touched[name]) {
            const error = validateField(name, value);
            setErrors(prev => ({ ...prev, [name]: error }));
        }
    }, [touched, validateField]);

    const handleBlur = useCallback((name) => {
        setTouched(prev => ({ ...prev, [name]: true }));
        const error = validateField(name, values[name]);
        setErrors(prev => ({ ...prev, [name]: error }));
    }, [validateField, values]);

    const resetForm = useCallback(() => {
        setValues(initialValues);
        setErrors({});
        setTouched({});
    }, [initialValues]);

    return {
        values,
        errors,
        touched,
        handleChange,
        handleBlur,
        validateAll,
        resetForm,
        setValues
    };
};

// Simplified validation rules
export const validationRules = {
    required: (value) => {
        const stringValue = String(value || '').trim();
        return !stringValue ? 'This field is required' : '';
    },
    
    email: (value) => {
        if (!value) return '';
        const emailRegex = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
        return !emailRegex.test(value) ? 'Please enter a valid email address' : '';
    },
    
    positive: (value) => {
        if (!value && value !== 0) return '';
        const numValue = parseFloat(value);
        return isNaN(numValue) || numValue <= 0 ? 'Value must be positive' : '';
    },
    
    minLength: (min) => (value) => {
        if (!value) return '';
        return value.length < min ? `Must be at least ${min} characters` : '';
    },
    
    maxLength: (max) => (value) => {
        if (!value) return '';
        return value.length > max ? `Must be no more than ${max} characters` : '';
    },
    
    // Indonesian specific validations
    indonesianTaxID: (value) => {
        if (!value) return '';
        const taxRegex = /^\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}$/;
        return !taxRegex.test(value) ? 'Invalid Tax ID format (XX.XXX.XXX.X-XXX.XXX)' : '';
    },
    
    accountCode: (value) => {
        if (!value) return '';
        const codeRegex = /^\d{4}$/;
        return !codeRegex.test(value) ? 'Account code must be 4 digits' : '';
    },
    
    indonesianPhone: (value) => {
        if (!value) return '';
        const phoneRegex = /^(\+62|62|0)[0-9]{8,12}$/;
        return !phoneRegex.test(value) ? 'Invalid Indonesian phone number format' : '';
    },
    
    currency: (value) => {
        if (!value && value !== 0) return '';
        const numValue = parseFloat(value);
        if (isNaN(numValue)) return 'Invalid currency amount';
        if (numValue < 0) return 'Currency amount cannot be negative';
        
        // Indonesian Rupiah doesn't use decimal places
        if (numValue !== Math.floor(numValue)) {
            return 'Indonesian Rupiah amounts should not have decimal places';
        }
        
        return '';
    },
};