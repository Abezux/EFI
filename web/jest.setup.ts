import '@testing-library/jest-dom';

jest.mock('next/navigation', () => ({
  useRouter: () => ({
    push: jest.fn(),
    replace: jest.fn(),
    prefetch: jest.fn(),
    back: jest.fn(),
    refresh: jest.fn(),
  }),
  useSearchParams: () => ({
    get: jest.fn().mockImplementation((key: string) => {
      if (key === 'q') return '';
      return null;
    }),
  }),
  usePathname: () => '/',
  notFound: jest.fn(),
}));
