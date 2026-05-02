/**
 * ViiB MediaHub - Like Button Component
 * 
 * Reusable heart/like button for songs that syncs with the backend.
 * Shows filled heart when liked, empty when not.
 * 
 * @module components/LikeButton
 */

import React from 'react';
import { Heart } from 'lucide-react';
import { useStore } from '../store';

interface LikeButtonProps {
    /** The song ID to toggle like for */
    songId: string;
    /** Size of the heart icon (default: 24) */
    size?: number;
    /** Additional CSS classes */
    className?: string;
    /** Show filled heart even when not liked (preview mode) */
    showFilled?: boolean;
}

/**
 * A heart button that toggles the liked status of a song.
 * Uses the Zustand store to check/update like status.
 */
export const LikeButton: React.FC<LikeButtonProps> = ({ 
    songId, 
    size = 24, 
    className = '',
    showFilled = false 
}) => {
    const { isLikedSong, toggleLikeSong } = useStore();
    const isLiked = isLikedSong(songId);

    const handleClick = async (e: React.MouseEvent) => {
        e.stopPropagation(); // Prevent triggering parent click handlers
        e.preventDefault();
        await toggleLikeSong(songId);
    };

    return (
        <button
            onClick={handleClick}
            className={`flex items-center justify-center min-h-[32px] min-w-[32px] transition-all hover:scale-110 ${
                isLiked || showFilled
                    ? 'text-error hover:text-error'
                    : 'text-text-subtle hover:text-error'
            } ${className}`}
            title={isLiked ? 'Unlike' : 'Like'}
            aria-label={isLiked ? 'Unlike song' : 'Like song'}
        >
            <Heart 
                size={size} 
                fill={isLiked || showFilled ? 'currentColor' : 'none'}
            />
        </button>
    );
};

export default LikeButton;
