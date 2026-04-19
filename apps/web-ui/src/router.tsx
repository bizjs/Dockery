import { createBrowserRouter, Navigate, type RouterNavigateOptions, type To } from 'react-router-dom';

import { MainLayout } from '@/layouts/main';
import { AuthGuard } from '@/components/common/AuthGuard';
import { ErrorPage } from '@/components/common';

import Catalog from '@/pages/Catalog';
import TagList from '@/pages/TagList';
import LoginPage from '@/pages/Login';
import UsersPage from '@/pages/Admin/Users';

export const router = createBrowserRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/',
    element: (
      <AuthGuard>
        <MainLayout />
      </AuthGuard>
    ),
    errorElement: <ErrorPage />,
    children: [
      { index: true, element: <Catalog /> },
      { path: 'tag-list/:image', element: <TagList /> },
      {
        path: 'admin/users',
        element: (
          <AuthGuard adminOnly>
            <UsersPage />
          </AuthGuard>
        ),
      },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
]);

export function navigate(to: To | null, opts?: RouterNavigateOptions) {
  router.navigate(to, opts);
}
