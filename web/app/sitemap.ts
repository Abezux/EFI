import { MetadataRoute } from 'next';
import { getEvents, getCategories } from '@/lib/api';
import { DEFAULT_SITE_URL, getCanonicalEventUrl } from '@/lib/seo';

export const dynamic = 'force-dynamic';
export const revalidate = 300; // revalidate every 5 minutes

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const baseUrl = DEFAULT_SITE_URL.replace(/\/$/, '');

  const staticRoutes: MetadataRoute.Sitemap = [
    {
      url: `${baseUrl}/`,
      lastModified: new Date(),
      changeFrequency: 'always',
      priority: 1.0,
    },
    {
      url: `${baseUrl}/search`,
      lastModified: new Date(),
      changeFrequency: 'daily',
      priority: 0.5,
    },
  ];

  let categoryRoutes: MetadataRoute.Sitemap = [];
  try {
    const categories = await getCategories({ isServer: true });
    categoryRoutes = categories.map((cat) => ({
      url: `${baseUrl}/category/${cat.slug}`,
      lastModified: new Date(),
      changeFrequency: 'hourly',
      priority: 0.8,
    }));
  } catch {
    // If API unavailable during build/fetch, continue
  }

  let eventRoutes: MetadataRoute.Sitemap = [];
  try {
    const eventsRes = await getEvents({ limit: 100, isServer: true });
    eventRoutes = eventsRes.events.map((event) => ({
      url: getCanonicalEventUrl(event, baseUrl),
      lastModified: new Date(event.last_updated_at || event.first_seen_at),
      changeFrequency: 'daily',
      priority: 0.7,
    }));
  } catch {
    // If API unavailable during build/fetch, continue
  }

  return [...staticRoutes, ...categoryRoutes, ...eventRoutes];
}
