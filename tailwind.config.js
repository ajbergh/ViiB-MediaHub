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
          // Modern Spotify Desktop neutral dark hierarchy
          0: '#000000', // Outer app frame / transport canvas
          1: '#121212', // Primary island surface (Sidebar, Main view, Queue)
          2: '#181818', // Elevated card / container surface
          3: '#242424', // Interactive control / input / divider surface
          highlight: '#2A2A2A', // Active nav / selected surface
          hover: '#1F1F1F', // Card & track row hover
          border: '#282828', // Subtle hairline divider
          slider: '#4D4D4D' // Unfilled slider track
        },
        text: {
          main: '#FFFFFF',
          secondary: '#B3B3B3',
          subtle: '#727272'
        },
        brand: {
          DEFAULT: '#1ED760',
          hover: '#3BE477'
        },
        accent: {
          purple: '#8B5CF6',
          green: '#1ED760',
          orange: '#FFA42B',
          blue: '#3D91F4',
          crimson: '#E91429'
        },
        warning: '#FFA42B',
        success: '#1ED760',
        error: '#F15E6C'
      },
      fontFamily: {
        sans: ['Inter', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
      },
      borderRadius: {
        xl: '8px'
      },
      fontSize: {
        display: ['32px', { lineHeight: '1.2', fontWeight: '700', letterSpacing: '-0.02em' }],
        section: ['22px', { lineHeight: '1.25', fontWeight: '700', letterSpacing: '-0.015em' }],
        card: ['16px', { lineHeight: '1.35', fontWeight: '600' }],
        body: ['14px', { lineHeight: '1.5', fontWeight: '400' }],
        meta: ['12px', { lineHeight: '1.4', fontWeight: '400' }]
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
