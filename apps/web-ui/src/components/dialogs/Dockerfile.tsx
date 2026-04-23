import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Copy } from 'lucide-react';
import { copyText } from '@bizjs/biz-utils';
import { toast } from 'sonner';

interface DockerfileProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  content?: string;
}

export function Dockerfile({ open, onOpenChange, content = '' }: DockerfileProps) {
  const handleCopy = async () => {
    try {
      // biz-utils falls back to the `document.execCommand('copy')`
      // path when navigator.clipboard is unavailable — relevant here
      // because Dockery ships on plain HTTP localhost:5001 by default
      // and the Clipboard API is gated behind secure contexts.
      await copyText(content);
      toast.success('Dockerfile copied');
    } catch {
      toast.error('Copy failed');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-center justify-between">
            Dockerfile
            <Button variant="ghost" size="icon" onClick={handleCopy}>
              <Copy className="h-4 w-4" />
            </Button>
          </DialogTitle>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-auto">
          <pre className="bg-muted p-4 rounded-md text-sm font-mono">
            {content || 'No Dockerfile content available'}
          </pre>
        </div>
      </DialogContent>
    </Dialog>
  );
}
