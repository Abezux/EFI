import React from 'react';
import { render, screen } from '@testing-library/react';
import { AiBadge, AiSummaryBox } from '../AiBadge';

describe('AiBadge & AiSummaryBox Transparency', () => {
  it('renders the AI-Generated Summary badge with required text', () => {
    render(<AiBadge />);
    const badge = screen.getByTestId('ai-summary-badge');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('AI-Generated Summary');
  });

  it('renders AiSummaryBox with badge when isAiGenerated is true', () => {
    render(
      <AiSummaryBox
        summary="Ethiopian National Bank issued new forex directives."
        isAiGenerated={true}
      />
    );

    expect(
      screen.getByText('Ethiopian National Bank issued new forex directives.')
    ).toBeInTheDocument();
    expect(screen.getByTestId('ai-summary-badge')).toBeInTheDocument();
    expect(
      screen.getByText(/Synthesized automatically from channel reports/i)
    ).toBeInTheDocument();
  });

  it('renders AiSummaryBox WITHOUT badge or disclaimer when isAiGenerated is false', () => {
    render(
      <AiSummaryBox
        summary="Manual editorial summary."
        isAiGenerated={false}
      />
    );

    expect(screen.getByText('Manual editorial summary.')).toBeInTheDocument();
    expect(screen.queryByTestId('ai-summary-badge')).not.toBeInTheDocument();
    expect(
      screen.queryByText(/Synthesized automatically from channel reports/i)
    ).not.toBeInTheDocument();
  });

  it('renders nothing when summary is empty', () => {
    const { container } = render(
      <AiSummaryBox summary="" isAiGenerated={true} />
    );
    expect(container.firstChild).toBeNull();
  });
});
