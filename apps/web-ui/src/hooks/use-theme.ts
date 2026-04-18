import { useEffect, useState } from 'react';
import { getCurrentTheme, setTheme as saveTheme, type ThemeMode } from '@/lib/theme';

export function useTheme() {
  const [theme, setThemeState] = useState<ThemeMode>(() => getCurrentTheme());

  useEffect(() => {
    const root = document.documentElement;
    root.classList.remove('light', 'dark');
    root.classList.add(theme);
  }, [theme]);

  const setTheme = (newTheme: 'light' | 'dark') => {
    setThemeState(newTheme);
    saveTheme(newTheme);
  };

  const toggleTheme = () => {
    setTheme(theme === 'light' ? 'dark' : 'light');
  };

  return { theme, setTheme, toggleTheme };
}
