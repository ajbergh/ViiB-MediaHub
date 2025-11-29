/**
 * ViiB MediaHub - Application Entry Point
 * 
 * Initializes the React application by mounting the App component
 * to the DOM root element. Uses React 19 with StrictMode enabled
 * for development warnings and double-render detection.
 * 
 * @module index
 */

import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';

const rootElement = document.getElementById('root');
if (!rootElement) {
  throw new Error("Could not find root element to mount to");
}

const root = ReactDOM.createRoot(rootElement);
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
