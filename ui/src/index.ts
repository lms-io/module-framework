import React from 'react';

// Example Component
const ModuleInfo: React.FC<{ name?: string }> = ({ name = "Module" }) => {
  return React.createElement('div', { 
    style: { padding: '16px', border: '1px solid #ccc', borderRadius: '8px' } 
  }, `Hello from ${name}!`);
};

// Example Settings Page
const SettingsPage = () => {
  return React.createElement('div', { style: { padding: '24px' } }, [
    React.createElement('h2', { key: 'h2' }, 'Module Settings'),
    React.createElement('p', { key: 'p' }, 'Configure your module parameters here.')
  ]);
};

// Standardized Plugin Contract
export const Plugin = {
  id: 'template-module', // Should match module.json id
  title: 'Template Module',
  pages: {
    'settings': SettingsPage,
  },
  components: {
    'Info': ModuleInfo
  }
};