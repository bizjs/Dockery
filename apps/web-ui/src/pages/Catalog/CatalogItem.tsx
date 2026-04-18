/**
 * CatalogElement Component
 * Individual Docker image card in the catalog
 */

import { Package, Tag } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Link } from 'react-router-dom';

interface CatalogItemProps {
  name: string;
  tagCount?: number;
  description?: string;
  onClick?: () => void;
}

export function CatalogItem({ name, tagCount, description, onClick }: CatalogItemProps) {
  const content = (
    <Card className="hover:shadow-lg transition-shadow cursor-pointer">
      <CardHeader>
        <div className="flex items-start gap-3">
          <Package className="h-6 w-6 text-primary mt-1" />
          <div className="flex-1 min-w-0">
            <CardTitle className="truncate">{name}</CardTitle>
            {description && <CardDescription className="mt-1 line-clamp-2">{description}</CardDescription>}
          </div>
        </div>
      </CardHeader>
      {tagCount !== undefined && (
        <CardContent>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Tag className="h-4 w-4" />
            <span>
              {tagCount} {tagCount === 1 ? 'tag' : 'tags'}
            </span>
          </div>
        </CardContent>
      )}
    </Card>
  );

  if (onClick) {
    return <div onClick={onClick}>{content}</div>;
  }

  return <Link to={`/tag-list/${encodeURIComponent(name)}`}>{content}</Link>;
}
