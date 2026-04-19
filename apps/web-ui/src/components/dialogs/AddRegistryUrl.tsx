import { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { addRegistryServers } from '@/lib/utils';

interface AddRegistryUrlProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNotify: (message: string, isError?: boolean) => void;
}

export function AddRegistryUrl({ open, onOpenChange, onNotify }: AddRegistryUrlProps) {
  const [url, setUrl] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!url.trim()) {
      onNotify('Please enter a registry URL', true);
      return;
    }
    try {
      addRegistryServers(url);
      onNotify(`Registry ${url} added successfully`);
      setUrl('');
      onOpenChange(false);
    } catch (error) {
      onNotify('Failed to add registry', true);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Registry URL</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="registry-url">Registry URL</Label>
              <Input
                id="registry-url"
                placeholder="https://registry.example.com"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit">Add</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
