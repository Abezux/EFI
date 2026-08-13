'use client';

import React, { createContext, useContext, useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { AdminUser, getAdminMe, getAdminCSRF, adminLogout } from '@/lib/api';

interface AdminAuthContextType {
  user: AdminUser | null;
  csrfToken: string;
  isLoading: boolean;
  logout: () => Promise<void>;
  refreshAuth: () => Promise<void>;
}

const AdminAuthContext = createContext<AdminAuthContextType>({
  user: null,
  csrfToken: '',
  isLoading: true,
  logout: async () => {},
  refreshAuth: async () => {},
});

export function AdminAuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<AdminUser | null>(null);
  const [csrfToken, setCsrfToken] = useState<string>('');
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const router = useRouter();
  const pathname = usePathname();

  const isLoginPage = pathname === '/admin/login';

  const refreshAuth = async () => {
    try {
      const [meRes, csrfRes] = await Promise.all([getAdminMe(), getAdminCSRF()]);
      setUser(meRes.user);
      setCsrfToken(csrfRes.csrf_token);
      if (isLoginPage) {
        router.push('/admin');
      }
    } catch {
      setUser(null);
      setCsrfToken('');
      if (!isLoginPage) {
        router.push('/admin/login');
      }
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    refreshAuth();
  }, [pathname]);

  const logout = async () => {
    try {
      await adminLogout();
    } finally {
      setUser(null);
      setCsrfToken('');
      router.push('/admin/login');
    }
  };

  return (
    <AdminAuthContext.Provider value={{ user, csrfToken, isLoading, logout, refreshAuth }}>
      {children}
    </AdminAuthContext.Provider>
  );
}

export function useAdminAuth() {
  return useContext(AdminAuthContext);
}
