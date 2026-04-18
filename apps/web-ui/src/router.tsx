import { createBrowserRouter, Navigate, type RouterNavigateOptions, type To } from 'react-router-dom';
import { MainLayout } from '@/layouts/main';
import Catalog from '@/pages/Catalog';
import TagList from '@/pages/TagList';
import { ErrorPage } from '@/components/common';

export const router = createBrowserRouter([
  {
    path: '/',
    element: <MainLayout />,
    errorElement: <ErrorPage />,
    children: [
      {
        index: true,
        element: <Catalog />,
      },
      {
        path: 'tag-list/:image',
        element: <TagList />,
      },
      {
        path: '*',
        element: <Navigate to="/" replace />,
      },
    ],
  },
]);

export function navigate(to: To | null, opts?: RouterNavigateOptions) {
  router.navigate(to, opts);
}
