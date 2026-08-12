import React from 'react';
import Link from 'next/link';
import { Category } from '@/lib/api';

interface CategoryNavProps {
  categories: Category[];
  activeSlug?: string;
  totalEvents?: number;
}

export function CategoryNav({
  categories,
  activeSlug,
  totalEvents,
}: CategoryNavProps) {
  const isAllActive = !activeSlug;

  return (
    <nav className="category-nav-wrapper" aria-label="News Categories">
      <div className="category-nav-list">
        <Link
          href="/"
          className={`category-nav-item ${isAllActive ? 'active' : ''}`}
          data-testid="category-nav-all"
        >
          <span>All Topics</span>
          {totalEvents !== undefined && (
            <span className="category-count">{totalEvents}</span>
          )}
        </Link>

        {categories.map((cat) => {
          const isActive = activeSlug === cat.slug;
          return (
            <Link
              key={cat.id}
              href={`/category/${cat.slug}`}
              className={`category-nav-item ${isActive ? 'active' : ''}`}
              data-testid={`category-nav-${cat.slug}`}
            >
              <span>{cat.name}</span>
              {cat.event_count !== undefined && cat.event_count > 0 && (
                <span className="category-count">{cat.event_count}</span>
              )}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
