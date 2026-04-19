/**
 * TagDetailDrawer Component
 * Drawer to display detailed information about a Docker image tag
 */

import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import { formatBinarySize } from '@/utils';
import { Copy, Check, X } from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import type { ImageInfo } from '@/services/registry.service';

interface TagDetailDrawerProps {
  open: boolean;
  onClose: () => void;
  tagInfo: ImageInfo | null;
  imageName: string;
}

export function TagDetailDrawer({ open, onClose, tagInfo, imageName }: TagDetailDrawerProps) {
  const [copiedField, setCopiedField] = useState<string | null>(null);

  if (!tagInfo) {
    return null;
  }

  const copyToClipboard = (text: string, fieldName: string) => {
    navigator.clipboard.writeText(text);
    setCopiedField(fieldName);
    toast.success(`${fieldName} copied to clipboard!`);
    setTimeout(() => setCopiedField(null), 2000);
  };

  const pullCommand = `docker pull ${imageName}:${tagInfo.tag}`;

  return (
    <Sheet open={open} onOpenChange={onClose}>
      <SheetContent className="w-full sm:max-w-4xl flex flex-col p-0">
        <SheetHeader className="px-6 py-4 border-b sticky top-0 bg-background z-10 flex flex-row items-center justify-between">
          <SheetTitle>
            Tag Details: {imageName}:{tagInfo.tag}
          </SheetTitle>
          <Button variant="ghost" size="icon" onClick={onClose} className="h-6 w-6 rounded-full">
            <X className="h-4 w-4" />
          </Button>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto px-6 py-6 space-y-6">
          {/* Pull Command */}
          <div className="space-y-2">
            <label className="text-sm font-medium text-muted-foreground">Pull Command</label>
            <div className="flex items-center gap-2">
              <code className="flex-1 px-3 py-2 bg-muted rounded-md text-sm font-mono break-all">{pullCommand}</code>
              <Button variant="ghost" size="sm" onClick={() => copyToClipboard(pullCommand, 'Pull Command')}>
                {copiedField === 'Pull Command' ? (
                  <Check className="h-4 w-4 text-green-500" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </Button>
            </div>
          </div>

          {/* Grid layout for small fields */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {/* Created Date */}
            {tagInfo.created && (
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">Created</label>
                <div className="px-3 py-2 bg-muted rounded-md text-sm">
                  {new Date(tagInfo.created).toLocaleString()}
                </div>
              </div>
            )}

            {/* OS */}
            {tagInfo.os && (
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">OS</label>
                <div className="px-3 py-2 bg-muted rounded-md text-sm">{tagInfo.os}</div>
              </div>
            )}

            {/* Architecture */}
            {tagInfo.architecture && (
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">Architecture</label>
                <div className="px-3 py-2 bg-muted rounded-md text-sm">{tagInfo.architecture}</div>
              </div>
            )}

            {/* Working Dir */}
            {tagInfo.workingDir && (
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">Working Dir</label>
                <div className="px-3 py-2 bg-muted rounded-md text-sm font-mono">{tagInfo.workingDir}</div>
              </div>
            )}

            {/* Exposed Ports */}
            {tagInfo.exposedPorts && tagInfo.exposedPorts.length > 0 && (
              <div className="space-y-2">
                <label className="text-sm font-medium text-muted-foreground">Exposed Ports</label>
                <div className="px-3 py-2 bg-muted rounded-md text-sm">{tagInfo.exposedPorts.join(', ')}</div>
              </div>
            )}

            {/* Layers */}
            <div className="space-y-2">
              <label className="text-sm font-medium text-muted-foreground">Layers</label>
              <div className="px-3 py-2 bg-muted rounded-md text-sm">
                {tagInfo.layers} {tagInfo.layers === 1 ? 'layer' : 'layers'}
              </div>
            </div>

            {/* Size */}
            <div className="space-y-2 md:col-span-2">
              <label className="text-sm font-medium text-muted-foreground">Size</label>
              <div className="px-3 py-2 bg-muted rounded-md text-sm">
                {formatBinarySize(tagInfo.size)}
                <span className="text-muted-foreground ml-2">({tagInfo.size.toLocaleString()} bytes)</span>
              </div>
            </div>
          </div>

          {/* ID */}
          {tagInfo.id && (
            <div className="space-y-2">
              <label className="text-sm font-medium text-muted-foreground">ID</label>
              <div className="flex items-center gap-2">
                <code className="flex-1 px-3 py-2 bg-muted rounded-md text-sm font-mono break-all">{tagInfo.id}</code>
                <Button variant="ghost" size="sm" onClick={() => copyToClipboard(tagInfo.id!, 'ID')}>
                  {copiedField === 'ID' ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                </Button>
              </div>
            </div>
          )}

          {/* Cmd */}
          {tagInfo.cmd && tagInfo.cmd.length > 0 && (
            <div className="space-y-2">
              <label className="text-sm font-medium text-muted-foreground">Cmd</label>
              <div className="px-3 py-2 bg-muted rounded-md text-sm">{tagInfo.cmd.join(' ')}</div>
            </div>
          )}

          {/* Env */}
          {tagInfo.env && tagInfo.env.length > 0 && (
            <div className="space-y-2">
              <label className="text-sm font-medium text-muted-foreground">Env</label>
              <div className="px-3 py-2 bg-muted rounded-md text-sm space-y-1">
                {tagInfo.env.map((envVar, index) => (
                  <div key={index} className="font-mono text-xs">
                    {envVar}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Labels */}
          {tagInfo.labels && Object.keys(tagInfo.labels).length > 0 && (
            <div className="space-y-2">
              <label className="text-sm font-medium text-muted-foreground">Labels</label>
              <div className="px-3 py-2 bg-muted rounded-md text-sm space-y-1">
                {Object.entries(tagInfo.labels).map(([key, value]) => (
                  <div key={key} className="font-mono text-xs">
                    <span className="text-primary">{key}</span>: {value}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Digest */}
          <div className="space-y-2">
            <label className="text-sm font-medium text-muted-foreground">Digest</label>
            <div className="flex items-center gap-2">
              <code className="flex-1 px-3 py-2 bg-muted rounded-md text-sm font-mono break-all">
                {tagInfo.digest || 'N/A'}
              </code>
              {tagInfo.digest && (
                <Button variant="ghost" size="sm" onClick={() => copyToClipboard(tagInfo.digest, 'Digest')}>
                  {copiedField === 'Digest' ? (
                    <Check className="h-4 w-4 text-green-500" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                </Button>
              )}
            </div>
          </div>

          {/* History */}
          {tagInfo.history && tagInfo.history.length > 0 && (
            <div className="space-y-2">
              <label className="text-sm font-medium text-muted-foreground">History</label>
              <div className="space-y-3">
                {tagInfo.history.map((item, index) => (
                  <div key={index} className="border rounded-md p-3 bg-muted/50 space-y-2">
                    {item.created && (
                      <div className="text-xs">
                        <span className="font-medium">Created:</span> {new Date(item.created).toLocaleString()}
                      </div>
                    )}
                    {item.created_by && (
                      <div className="text-xs">
                        <span className="font-medium">Created By:</span>{' '}
                        <code className="text-xs break-all">{item.created_by}</code>
                      </div>
                    )}
                    {item.size !== undefined && item.size > 0 && (
                      <div className="text-xs">
                        <span className="font-medium">Size:</span> {formatBinarySize(item.size)}
                      </div>
                    )}
                    {item.id && (
                      <div className="text-xs">
                        <span className="font-medium">ID:</span> <code className="text-xs break-all">{item.id}</code>
                      </div>
                    )}
                    {item.comment && (
                      <div className="text-xs">
                        <span className="font-medium">Comment:</span> {item.comment}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Actions */}
        {/* <div className=" px-6 flex gap-3 sticky bottom-0 bg-background py-4 border-t">
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
        </div> */}
      </SheetContent>
    </Sheet>
  );
}
