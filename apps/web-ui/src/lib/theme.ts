export interface ThemeColors {
  'primary-text': string;
  'neutral-text': string;
  background: string;
  'hover-background': string;
  'accent-text': string;
  'header-text': string;
  'header-accent-text': string;
  'header-background': string;
  'footer-text': string;
  'footer-neutral-text': string;
  'footer-background': string;
}

const LIGHT_THEME: ThemeColors = {
  'primary-text': '#25313b',
  'neutral-text': '#777777',
  background: '#ffffff',
  'hover-background': '#eeeeee',
  'accent-text': '#5f7796',
  'header-text': '#ffffff',
  'header-accent-text': '#7b9ac2',
  'header-background': '#25313b',
  'footer-text': '#ffffff',
  'footer-neutral-text': '#adbacd',
  'footer-background': '#344251',
};

const DARK_THEME: ThemeColors = {
  'primary-text': '#98a8bd',
  'neutral-text': '#6d7fab',
  background: '#22272e',
  'hover-background': '#343a4b',
  'accent-text': '#5c88ff',
  'header-text': '#ffffff',
  'header-accent-text': '#7ea1ff',
  'header-background': '#333a45',
  'footer-text': '#ffffff',
  'footer-neutral-text': '#98afcf',
  'footer-background': '#344251',
};

const LOCAL_STORAGE_THEME = 'registryUiTheme';

export type ThemeMode = 'light' | 'dark' | 'auto' | '';

export interface ThemeProps {
  theme?: ThemeMode;
  [key: string]: string | undefined;
}

/**
 * Normalize theme property key
 */
const normalizeKey = (k: string): string =>
  k
    .replace(/([A-Z])/g, '-$1')
    .toLowerCase()
    .replace(/^theme-/, '');

/**
 * Check if dark mode should be preferred based on theme setting
 */
const preferDarkMode = ({ theme }: ThemeProps): boolean => {
  if (theme === 'auto' || theme === '') {
    switch (localStorage.getItem(LOCAL_STORAGE_THEME)) {
      case 'dark':
        return true;
      case 'light':
        return false;
      default:
        if (typeof window.matchMedia === 'function') {
          const prefersDarkScheme = window.matchMedia('(prefers-color-scheme: dark)');
          return prefersDarkScheme && prefersDarkScheme.matches;
        }
    }
  }
  return theme === 'dark';
};

/**
 * Load and apply theme to the document
 * @param props - Theme configuration props
 * @param style - CSSStyleDeclaration to apply theme variables to
 * @returns The applied theme mode ('light' or 'dark')
 */
export const loadTheme = (props: ThemeProps, style: CSSStyleDeclaration): ThemeMode => {
  const isDarkMode = preferDarkMode(props);
  const THEME: Partial<ThemeColors> = isDarkMode ? { ...DARK_THEME } : { ...LIGHT_THEME };

  // Apply custom theme overrides
  Object.entries(props)
    .filter(([k, v]) => v && /^theme[A-Z]/.test(k))
    .map(([k, v]) => [normalizeKey(k), v] as [string, string])
    .forEach(([k, v]) => {
      THEME[k as keyof ThemeColors] = v;
    });

  // Apply theme variables to CSS
  Object.entries(THEME).forEach(([k, v]) => {
    if (v) {
      style.setProperty(`--${k}`, v);
    }
  });

  const theme: ThemeMode = isDarkMode ? 'dark' : 'light';
  localStorage.setItem(LOCAL_STORAGE_THEME, theme);
  return theme;
};

/**
 * Get current theme from localStorage
 */
export const getCurrentTheme = (): ThemeMode => {
  const stored = localStorage.getItem(LOCAL_STORAGE_THEME);
  return (stored as ThemeMode) || 'auto';
};

/**
 * Set theme mode
 */
export const setTheme = (theme: ThemeMode): void => {
  localStorage.setItem(LOCAL_STORAGE_THEME, theme);
  // Apply theme to document root
  loadTheme({ theme }, document.documentElement.style);

  // Toggle dark class on html element for Tailwind
  if (theme === 'dark' || (theme === 'auto' && preferDarkMode({ theme }))) {
    document.documentElement.classList.add('dark');
  } else {
    document.documentElement.classList.remove('dark');
  }
};

/**
 * Initialize theme on app load
 */
export const initTheme = (): void => {
  const theme = getCurrentTheme();
  setTheme(theme);
};
