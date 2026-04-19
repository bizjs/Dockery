import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';

interface ConfirmDeleteImageProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNotify: (message: string, isError?: boolean) => void;
  imageName?: string;
  imageTag?: string;
  onConfirm?: () => void;
}

export function ConfirmDeleteImage({ open, onOpenChange, onNotify, imageName, imageTag, onConfirm }: ConfirmDeleteImageProps) {
  const handleConfirm = () => {
    if (onConfirm) {
      onConfirm();
    }
    onNotify(`Image ${imageName}:${imageTag} deleted`);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Confirm Delete</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete this image?
          </DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <p className="text-sm">
            <span className="font-semibold">Image:</span> {imageName}:{imageTag}
          </p>
          <p className="text-sm text-destructive mt-2">
            This action cannot be undone.
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button variant="destructive" onClick={handleConfirm}>Delete</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
