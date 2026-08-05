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
        'w-full rounded-full bg-surface-2 px-5 ring-surface-border shadow-lg shadow-black/10',
        compact ? 'h-11' : 'h-12',
        className
      )}
      inputClassName="text-base placeholder:text-text-secondary"
      leftIcon={<Search size={20} className="text-text-secondary" aria-hidden="true" />}
      rightIcon={
        <button
          type="button"
          onClick={submitSearch}
          className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-brand text-black transition-transform hover:scale-105 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/60 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-2 motion-reduce:transition-none motion-reduce:hover:transform-none"
          aria-label="Search"
        >
          <ArrowRight size={16} aria-hidden="true" />
        </button>
      }
      type="text"
      placeholder="Search songs, albums, artists"
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
