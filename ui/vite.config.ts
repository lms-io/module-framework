import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: path.resolve(__dirname, 'src/index.ts'),
      name: 'BundleTemplateUI',
      fileName: (format) => `bundle-template-ui.${format}.js`,
      formats: ['es']
    },
    rollupOptions: {
      // Externalize dependencies that the main dashboard will provide
      external: ['react', 'react-dom', '@mui/material', '@mui/icons-material'],
      output: {
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM'
        }
      }
    }
  }
})