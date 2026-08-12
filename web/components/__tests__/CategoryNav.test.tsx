import React from 'react';
import { render, screen } from '@testing-library/react';
import { CategoryNav } from '../CategoryNav';
import { Category } from '@/lib/api';

const mockCategories: Category[] = [
  { id: 1, name: 'Banking & Finance', slug: 'banking-finance', event_count: 5 },
  { id: 2, name: 'Inflation & Prices', slug: 'inflation-prices', event_count: 2 },
];

describe('CategoryNav Component', () => {
  it('renders "All Topics" and each category link', () => {
    render(<CategoryNav categories={mockCategories} totalEvents={7} />);

    expect(screen.getByTestId('category-nav-all')).toBeInTheDocument();
    expect(screen.getByText('Banking & Finance')).toBeInTheDocument();
    expect(screen.getByText('Inflation & Prices')).toBeInTheDocument();
  });

  it('marks active category correctly', () => {
    render(
      <CategoryNav
        categories={mockCategories}
        activeSlug="banking-finance"
        totalEvents={7}
      />
    );

    const activeItem = screen.getByTestId('category-nav-banking-finance');
    expect(activeItem).toHaveClass('active');

    const allItem = screen.getByTestId('category-nav-all');
    expect(allItem).not.toHaveClass('active');
  });
});
