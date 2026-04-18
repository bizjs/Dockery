import { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { getRegistryServers, setRegistryServers } from '@/lib/utils';

interface ChangeRegistryUrlProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNotify: (message: string, isError?: boolean) => void;
}

export function ChangeRegistryUrl({ open, onOpenChange, onNotify }: ChangeRegistryUrlProps) {
  const [selectedUrl, setSelectedUrl] = useState('');
  const registries = getRegistryServers() as string[];

  const handleChange = () => {
    if (!selectedUrl) {
      onNotify('Please select a registry', true);
      return;
    }
    const filtered = registries.filter(r => r !== selectedUrl);
    setRegistryServers([selectedUrl, ...filtered]);
    onNotify(`Switched to ${selectedUrl}`);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Change Registry URL</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <Select value={selectedUrl} onValueChange={setSelectedUrl}>
            <SelectTrigger>
              <SelectValue placeholder="Select a registry" />
            </SelectTrigger>
            <SelectContent>
              {registries.map((url) => (
                <SelectItem key={url} value={url}>{url}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={handleChange}>Change</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
