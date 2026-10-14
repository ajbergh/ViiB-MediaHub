/** Provides the Home search entry control. */

import React, { useState } from 'react';
import { ArrowRight, Search } from 'lucide-react';
import { useNavigate } from 'react-router';
import { TextInput } from '../ui/TextInput';
import { cn } from '../ui/cn';

interface HomeSearchBarProps {
  className?: string;
  compact?: boolean;
}

export const HomeSearchBar: React.FC<HomeSearchBarProps> = ({ className, compact = false }) => {
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');

  const submitSearch = () => {
    const query = searchQuery.trim();
    if (query) {
      navigate('/search', { state: { query } });
    } else {
      navigate('/search');
    }
  };

  return (
    <TextInput
      className={cn(
        'w-full rounded-full bg-surface-3 px-4 ring-1 ring-transparent hover:bg-surface-highlight hover:ring-surface-border',
        compact ? 'h-10' : 'h-11',
        className
      )}
      inputClassName="text-sm placeholder:text-text-subtle"
      leftIcon={<Search size={18} className="text-text-secondary" aria-hidden="true" />}
      rightIcon={
        searchQuery.trim() ? (
          <button
            type="button"
            onClick={submitSearch}
            className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-brand text-surface-0 transition-transform hover:scale-105 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/60"
            aria-label="Search"
          >
            <ArrowRight size={14} aria-hidden="true" />
          </button>
        ) : undefined
      }
      type="text"
      placeholder="What do you want to play?"
      aria-label="Search your library"
      value={searchQuery}
      onChange={(e) => setSearchQuery(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          submitSearch();
        }
      }}
    />
  );
};
