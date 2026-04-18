import { useState, useCallback } from 'react';
import { getRegistryServers } from '@/lib/utils';

export function useDockerRegistry() {
  const [registryUrl, setRegistryUrl] = useState<string>(() => {
    const servers = getRegistryServers() as string[];
    return servers[0] || '';
  });

  const [isSecured, setIsSecured] = useState(false);

  const changeRegistry = useCallback((url: string) => {
    setRegistryUrl(url);
  }, []);

  return { registryUrl, isSecured, setIsSecured, changeRegistry };
}
