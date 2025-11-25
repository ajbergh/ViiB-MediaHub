import React from 'react';
import { Download } from 'lucide-react';

export const Downloads: React.FC = () => {
  return (
    <div className="p-8 pb-32 flex flex-col items-center justify-center min-h-[60vh]">
        <div className="mb-6 opacity-20">
            <Download size={80} />
        </div>
        <h1 className="text-2xl font-bold mb-2">No Downloads Yet</h1>
        <p className="text-[#6f7480] text-center max-w-sm">
            Download songs, albums, or playlists to listen offline. Look for the download button on any song.
        </p>
    </div>
  );
};
