export interface AppConfig {
  registryUrl: string;
  catalogElementsLimit: number;
  useControlCacheHeader: boolean;
  singleRegistry: boolean;
  showContentDigest: boolean;
  showCatalogNbTags: boolean;
  historyCustomLabels: Record<string, string>;
  defaultRegistries: string[];
}

const defaultConfig: AppConfig = {
  registryUrl: import.meta.env.VITE_REGISTRY_URL || window.location.origin,
  catalogElementsLimit: 100,
  useControlCacheHeader: false,
  singleRegistry: false,
  showContentDigest: true,
  showCatalogNbTags: true,
  historyCustomLabels: {},
  defaultRegistries: [],
};

export const config: AppConfig = {
  ...defaultConfig,
  registryUrl: import.meta.env.VITE_REGISTRY_URL || defaultConfig.registryUrl,
  catalogElementsLimit: Number(import.meta.env.VITE_CATALOG_ELEMENTS_LIMIT) || defaultConfig.catalogElementsLimit,
  useControlCacheHeader: import.meta.env.VITE_USE_CONTROL_CACHE_HEADER === 'true',
  singleRegistry: import.meta.env.VITE_SINGLE_REGISTRY === 'true',
  showContentDigest: import.meta.env.VITE_SHOW_CONTENT_DIGEST !== 'false',
  showCatalogNbTags: import.meta.env.VITE_SHOW_CATALOG_NB_TAGS !== 'false',
};
