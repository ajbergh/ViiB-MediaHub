/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
    "./components/**/*.{js,ts,jsx,tsx}",
    "./pages/**/*.{js,ts,jsx,tsx}",
    "./App.tsx",
  ],
  theme: {
    extend: {
      colors: {
        surface: {
          // Viib Design System v1
          0: '#0B0B0E', // Background
          1: '#121216', // Surface Dark
          2: '#18181E', // Surface Raised
          3: '#24242B', // Dividers
          // Back-compat aliases used throughout the app.
          highlight: '#18181E',
          hover: '#18181E',
          border: '#24242B',
          slider: '#24242B'
        },
        text: {
          main: 'rgba(255,255,255,0.9)',
          secondary: '#B8BAC6',
          subtle: '#7A7D8C'
        },
        brand: {
          // Viib Purple (Brand / AI DJ)
          DEFAULT: '#9B5CFF',
          hover: '#9B5CFF'
        },
        accent: {
          // Accent Colors (max 2 per screen; use intentionally)
          purple: '#9B5CFF',
          green: '#3EE089',
          orange: '#FF9F43',
          blue: '#4EA1FF',
          crimson: '#FF5D5D'
        },
        // Status colors map to v1 accents
        warning: '#FF9F43',
        success: '#3EE089',
        error: '#FF5D5D'
      },
      fontFamily: {
        sans: ['Inter', 'sans-serif'],
      },
      borderRadius: {
        // Cards: 12px
        xl: '12px'
      },
      fontSize: {
        // Viib Type Scale
        display: ['36px', { lineHeight: '1.25', fontWeight: '600' }],
        section: ['24px', { lineHeight: '1.25', fontWeight: '500' }],
        card: ['18px', { lineHeight: '1.25', fontWeight: '500' }],
        body: ['15px', { lineHeight: '1.5', fontWeight: '400' }],
        meta: ['13px', { lineHeight: '1.5', fontWeight: '400' }]
      },
      keyframes: {
        shimmer: {
          '100%': { transform: 'translateX(100%)' }
        },
        'fade-in': {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        'slide-up': {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        'slide-in-right': {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        'scale-in': {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        // Subtle, slow background loop (20–40s) for allowed use-cases.
        'bg-loop': {
          '0%': { backgroundPosition: '0% 50%' },
          '50%': { backgroundPosition: '100% 50%' },
          '100%': { backgroundPosition: '0% 50%' }
        }
      },
      animation: {
        shimmer: 'shimmer 2s infinite',
        'fade-in': 'fade-in 0.3s ease-out',
        'slide-up': 'slide-up 0.3s ease-out',
        'slide-in-right': 'slide-in-right 0.3s ease-out',
        'scale-in': 'scale-in 0.2s ease-out',
        'bg-loop': 'bg-loop 30s ease-out infinite'
      },
      transitionTimingFunction: {
        // Enforce v1 easing default.
        DEFAULT: 'ease-out'
      }
    }
  },
  plugins: [],
}
