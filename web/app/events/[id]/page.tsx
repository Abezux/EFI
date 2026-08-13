import { notFound, permanentRedirect } from 'next/navigation';
import { getEventById } from '@/lib/api';
import { getCanonicalEventPath } from '@/lib/seo';

export const dynamic = 'force-dynamic';
export const revalidate = 0;

interface LegacyEventPageProps {
  params: {
    id: string;
  };
}

export default async function LegacyEventPage({ params }: LegacyEventPageProps) {
  const eventId = parseInt(params.id, 10);
  if (Number.isNaN(eventId) || eventId <= 0) {
    notFound();
  }

  let event = null;
  try {
    event = await getEventById(eventId, { isServer: true });
  } catch {
    notFound();
  }

  if (!event) {
    notFound();
  }

  // 301 Permanent Redirect to canonical URL
  permanentRedirect(getCanonicalEventPath(event));
}
