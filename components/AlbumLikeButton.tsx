/**
 * ViiB MediaHub - Album Like Button Component
 * 
 * Reusable heart/like button for albums that syncs with the backend.
 * Shows filled heart when liked, empty when not.
 * 
 * @module components/AlbumLikeButton
 */

import React from 'react';
import { Heart } from 'lucide-react';
import { useStore } from '../store';

interface AlbumLikeButtonProps {
    /** The album key (format: "AlbumName::ArtistName") to toggle like for */
    albumKey: string;
    /** Size of the heart icon (default: 24) */
    size?: number;
    /** Additional CSS classes */
    className?: string;
    /** Show filled heart even when not liked (preview mode) */
    showFilled?: boolean;
}

/**
 * A heart button that toggles the liked status of an album.
 * Uses the Zustand store to check/update like status.
 */
export const AlbumLikeButton: React.FC<AlbumLikeButtonProps> = ({ 
    albumKey, 
    size = 24, 
    className = '',
    showFilled = false 
}) => {
    const { isLikedAlbum, toggleLikeAlbum } = useStore();
    const isLiked = isLikedAlbum(albumKey);

    const handleClick = async (e: React.MouseEvent) => {
        e.stopPropagation(); // Prevent triggering parent click handlers
        e.preventDefault();
        await toggleLikeAlbum(albumKey);
    };

    return (
        <button
            onClick={handleClick}
            className={`transition-all hover:scale-110 ${
                isLiked || showFilled
                    ? 'text-error hover:text-error'
                    : 'text-text-subtle hover:text-error'
            } ${className}`}
            title={isLiked ? 'Unlike' : 'Like'}
            aria-label={isLiked ? 'Unlike album' : 'Like album'}
        >
            <Heart 
                size={size} 
                fill={isLiked || showFilled ? 'currentColor' : 'none'}
            />
        </button>
    );
};

export default AlbumLikeButton;
