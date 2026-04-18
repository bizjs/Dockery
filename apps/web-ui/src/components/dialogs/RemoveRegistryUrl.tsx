import { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { getRegistryServers, removeRegistryServers } from '@/lib/utils';

interface RemoveRegistryUrlProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNotify: (message: string, isError?: boolean) => void;
}

export function RemoveRegistryUrl({ open, onOpenChange, onNotify }: RemoveRegistryUrlProps) {
  const [selectedUrl, setSelectedUrl] = useState('');
  const registries = getRegistryServers() as string[];

  const handleRemove = () => {
    if (!selectedUrl) {
      onNotify('Please select a registry', true);
      return;
    }
    removeRegistryServers(selectedUrl);
    onNotify(`Registry ${selectedUrl} removed`);
    setSelectedUrl('');
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Remove Registry URL</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <Select value={selectedUrl} onValueChange={setSelectedUrl}>
            <SelectTrigger>
              <SelectValue placeholder="Select a registry to remove" />
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
          <Button variant="destructive" onClick={handleRemove}>Remove</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
