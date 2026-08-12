'use client';

import React, { useState, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Search as SearchIcon } from 'lucide-react';

interface SearchBarProps {
  initialQuery?: string;
  placeholder?: string;
}

export function SearchBar({
  initialQuery = '',
  placeholder = 'Search Ethiopian news in English or Amharic (e.g. CBE, የውጭ ምንዛሬ)...',
}: SearchBarProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [query, setQuery] = useState(initialQuery);

  useEffect(() => {
    const q = searchParams.get('q');
    if (q !== null) {
      setQuery(q);
    }
  }, [searchParams]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = query.trim();
    if (trimmed) {
      router.push(`/search?q=${encodeURIComponent(trimmed)}`);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="search-form" role="search">
      <SearchIcon size={18} className="search-icon" aria-hidden="true" />
      <input
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={placeholder}
        className="search-input"
        data-testid="search-input"
        aria-label="Search news events"
      />
    </form>
  );
}
