import { useState } from 'react';
import { AddRegistryUrl } from './AddRegistryUrl';
import { ChangeRegistryUrl } from './ChangeRegistryUrl';
import { RemoveRegistryUrl } from './RemoveRegistryUrl';
import { ConfirmDeleteImage } from './ConfirmDeleteImage';
import { Dockerfile } from './Dockerfile';

interface DialogsMenuProps {
  onNotify: (message: string, isError?: boolean) => void;
}

export function DialogsMenu({ onNotify }: DialogsMenuProps) {
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [changeDialogOpen, setChangeDialogOpen] = useState(false);
  const [removeDialogOpen, setRemoveDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [dockerfileDialogOpen, setDockerfileDialogOpen] = useState(false);

  return (
    <>
      <AddRegistryUrl open={addDialogOpen} onOpenChange={setAddDialogOpen} onNotify={onNotify} />
      <ChangeRegistryUrl open={changeDialogOpen} onOpenChange={setChangeDialogOpen} onNotify={onNotify} />
      <RemoveRegistryUrl open={removeDialogOpen} onOpenChange={setRemoveDialogOpen} onNotify={onNotify} />
      <ConfirmDeleteImage open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen} onNotify={onNotify} />
      <Dockerfile open={dockerfileDialogOpen} onOpenChange={setDockerfileDialogOpen} />
    </>
  );
}
