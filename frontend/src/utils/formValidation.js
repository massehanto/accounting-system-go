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
    
    minLength: (min) => (value) => {
        if (!value) return '';
        return value.length < min ? `Must be at least ${min} characters` : '';
    },
    
    accountCode: (value) => {
        if (!value) return '';
        const codeRegex = /^\d{4}$/;
        return !codeRegex.test(value) ? 'Account code must be 4 digits' : '';
    },
};