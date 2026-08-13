import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import AdminLoginPage from '../admin/login/page';
import AdminDashboardPage from '../admin/page';
import AdminChannelsPage from '../admin/channels/page';
import AdminEventsPage from '../admin/events/page';
import AdminReviewQueuePage from '../admin/review-queue/page';
import * as api from '@/lib/api';

// Mock useRouter and usePathname
const mockPush = jest.fn();
jest.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: jest.fn(),
    refresh: jest.fn(),
  }),
  usePathname: () => '/admin',
  useParams: () => ({ id: '101' }),
}));

import * as AdminAuth from '@/app/admin/AdminAuthContext';

describe('Admin Panel Pages Integration', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(AdminAuth, 'useAdminAuth').mockReturnValue({
      user: { id: 1, email: 'admin@example.com', role: 'admin' },
      csrfToken: 'test-csrf-token-12345',
      isLoading: false,
      logout: jest.fn(),
      refreshAuth: jest.fn(),
    });
  });


  describe('AdminLoginPage', () => {
    it('renders login form and authenticates admin user', async () => {
      jest.spyOn(api, 'adminLogin').mockResolvedValueOnce({
        user: { id: 1, email: 'admin@example.com', role: 'admin' },
        csrf_token: 'token-abc',
      });

      render(<AdminLoginPage />);

      expect(screen.getByText('Admin Console')).toBeInTheDocument();
      expect(screen.getByLabelText('Email Address')).toBeInTheDocument();
      expect(screen.getByLabelText('Password')).toBeInTheDocument();

      fireEvent.change(screen.getByLabelText('Email Address'), {
        target: { value: 'admin@example.com' },
      });
      fireEvent.change(screen.getByLabelText('Password'), {
        target: { value: 'SecurePass123!' },
      });

      fireEvent.click(screen.getByRole('button', { name: 'Sign In' }));

      await waitFor(() => {
        expect(api.adminLogin).toHaveBeenCalledWith('admin@example.com', 'SecurePass123!');
      });
    });
  });

  describe('AdminDashboardPage', () => {
    it('renders high-level metric cards and review queue preview', async () => {
      jest.spyOn(api, 'getAdminChannels').mockResolvedValueOnce({
        channels: [
          {
            id: 1,
            telegram_channel_id: 1001,
            name: 'Tikvah Ethiopia',
            handle: 'tikvahethiopia',
            is_active: true,
            added_at: '2026-08-01T00:00:00Z',
            post_count: 42,
          },
        ],
      });
      jest.spyOn(api, 'getAdminEvents')
        .mockResolvedValueOnce({ events: [], total: 15, limit: 1, offset: 0 }) // total events
        .mockResolvedValueOnce({ events: [], total: 2, limit: 1, offset: 0 }); // hidden events
      jest.spyOn(api, 'getReviewQueue').mockResolvedValueOnce({
        posts: [
          {
            raw_post_id: 99,
            channel_id: 1,
            channel_name: 'Tikvah Ethiopia',
            telegram_message_id: 555,
            raw_text: 'Uncertain border trade agreement update',
            posted_at: '2026-08-11T12:00:00Z',
            ingested_at: '2026-08-11T12:05:00Z',
            candidate_event_id: 10,
            candidate_event_title: 'Trade Policy Reform',
          },
        ],
        total: 1,
        limit: 5,
        offset: 0,
      });

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Dashboard Overview')).toBeInTheDocument();
        expect(screen.getByText(/1 active/)).toBeInTheDocument();
        expect(screen.getByText('15')).toBeInTheDocument();
        expect(screen.getByText('2')).toBeInTheDocument();
        expect(screen.getByText('Uncertain border trade agreement update')).toBeInTheDocument();
      });
    });
  });

  describe('AdminChannelsPage', () => {
    it('renders channel list with active badges and pause action', async () => {
      jest.spyOn(api, 'getAdminChannels').mockResolvedValueOnce({
        channels: [
          {
            id: 1,
            telegram_channel_id: 1001,
            name: 'Fana Broadcasting',
            handle: 'fanatelevision',
            is_active: true,
            added_at: '2026-08-01T00:00:00Z',
            post_count: 120,
          },
        ],
      });

      render(<AdminChannelsPage />);

      await waitFor(() => {
        expect(screen.getByText('Channel Management')).toBeInTheDocument();
        expect(screen.getByText('Fana Broadcasting')).toBeInTheDocument();
        expect(screen.getByText(/fanatelevision/)).toBeInTheDocument();
        expect(screen.getByText('Pause Feed')).toBeInTheDocument();
      });
    });
  });

  describe('AdminEventsPage', () => {
    it('renders events list with takedown controls', async () => {
      jest.spyOn(api, 'getAdminEvents').mockResolvedValueOnce({
        events: [
          {
            id: 101,
            canonical_title: 'National Bank FX Directive',
            ai_headline: 'National Bank FX Directive Issued',
            slug: 'national-bank-fx-directive',
            ai_summary: 'Directive issued by NBE.',
            status: 'active',
            is_hidden: false,
            source_count: 3,
            first_seen_at: '2026-08-11T12:00:00Z',
            last_updated_at: '2026-08-11T12:00:00Z',
          },
        ],
        total: 1,
        limit: 20,
        offset: 0,
      });

      render(<AdminEventsPage />);

      await waitFor(() => {
        expect(screen.getByText('Event Moderation & Takedowns')).toBeInTheDocument();
        expect(screen.getByText('National Bank FX Directive Issued')).toBeInTheDocument();
        expect(screen.getByText('Take Down')).toBeInTheDocument();
      });
    });
  });

  describe('AdminReviewQueuePage', () => {
    it('renders ambiguous posts with decision actions', async () => {
      jest.spyOn(api, 'getReviewQueue').mockResolvedValueOnce({
        posts: [
          {
            raw_post_id: 501,
            channel_id: 1,
            channel_name: 'Reporter Ethiopia',
            telegram_message_id: 777,
            raw_text: 'Border tariff adjustments announced by customs authority.',
            posted_at: '2026-08-11T12:00:00Z',
            ingested_at: '2026-08-11T12:05:00Z',
            candidate_event_id: 20,
            candidate_event_title: 'Customs Policy',
            ai_run_reason: 'Similarity 0.72 in ambiguous band',
          },
        ],
        total: 1,
        limit: 20,
        offset: 0,
      });

      render(<AdminReviewQueuePage />);

      await waitFor(() => {
        expect(screen.getByText('Ambiguous Review Queue')).toBeInTheDocument();
        expect(screen.getByText('Border tariff adjustments announced by customs authority.')).toBeInTheDocument();
        expect(screen.getByText('Attach to Event')).toBeInTheDocument();
        expect(screen.getByText('Create New Story')).toBeInTheDocument();
        expect(screen.getByText('Discard / Spam')).toBeInTheDocument();
      });
    });
  });
});
