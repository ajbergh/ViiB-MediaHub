import { useEffect, useMemo, useState } from 'react';
import {
  deleteDJMixIdea,
  djMixIdeaKey,
  listDJMixIdeas,
  saveDJMixIdea,
  type MixIdeaStorage,
  type NewDJMixIdea,
  type SavedDJMixIdea,
} from '../../../lib/djMixIdeas';

interface DJSavedMixIdeasProps {
  currentIdea: NewDJMixIdea | null;
}

function browserStorage(): MixIdeaStorage | null {
  try {
    return typeof window === 'undefined' ? null : window.localStorage;
  } catch {
    return null;
  }
}

export function DJSavedMixIdeas({ currentIdea }: DJSavedMixIdeasProps) {
  const [ideas, setIdeas] = useState<SavedDJMixIdea[]>([]);
  const [message, setMessage] = useState('');
  const currentKey = useMemo(() => currentIdea
    ? `${currentIdea.source.trackId}\0${currentIdea.candidate.trackId}\0${currentIdea.intent}` : null, [currentIdea]);
  const alreadySaved = !!currentKey && ideas.some(idea => djMixIdeaKey(idea) === currentKey);

  useEffect(() => {
    const storage = browserStorage();
    if (storage) setIdeas(listDJMixIdeas(storage));
  }, []);

  const save = () => {
    const storage = browserStorage();
    if (!storage || !currentIdea) {
      setMessage('Saved ideas are unavailable in this browser session.');
      return;
    }
    try {
      setIdeas(saveDJMixIdea(storage, currentIdea));
      setMessage('Mix idea saved on this device.');
    } catch {
      setMessage('Could not save the mix idea in browser storage.');
    }
  };

  const remove = (idea: SavedDJMixIdea) => {
    const storage = browserStorage();
    if (!storage) {
      setMessage('Saved ideas are unavailable in this browser session.');
      return;
    }
    try {
      setIdeas(deleteDJMixIdea(storage, djMixIdeaKey(idea)));
      setMessage('Mix idea removed.');
    } catch {
      setMessage('Could not update browser storage.');
    }
  };

  return <section aria-label="Saved Mix Next ideas" className="mt-2 space-y-1 text-neutral-300">
    <div className="flex items-center gap-2">
      <button type="button" disabled={!currentIdea || alreadySaved} onClick={save}
        title="Save a local snapshot of this recommendation and its evidence; no audio or deck state is stored."
        className="rounded border border-cyan-500/40 px-2 py-1 text-cyan-200 disabled:cursor-not-allowed disabled:opacity-50">
        {alreadySaved ? 'Idea saved' : 'Save mix idea'}
      </button>
      <span className="text-neutral-500">{ideas.length} saved on this device</span>
      {message && <span role="status" className="text-neutral-400">{message}</span>}
    </div>
    {ideas.length > 0 && <ul className="space-y-1">
      {ideas.map(idea => <li key={djMixIdeaKey(idea)} className="flex flex-wrap items-center gap-x-2 rounded border border-neutral-800 px-2 py-1">
        <span>{idea.source.title} — {idea.source.artist}</span>
        <span aria-hidden="true">→</span>
        <span>{idea.candidate.title} — {idea.candidate.artist}</span>
        <span className="text-neutral-500">{idea.intent} · {Math.round(idea.score * 100)}% · {new Date(idea.createdAt).toLocaleDateString()}</span>
        <button type="button" onClick={() => remove(idea)} aria-label={`Remove ${idea.source.title} to ${idea.candidate.title} mix idea`}
          className="ml-auto rounded border border-neutral-700 px-1 text-neutral-300">Remove</button>
      </li>)}
    </ul>}
    <p className="text-[10px] text-neutral-500">Saved ideas keep recommendation evidence and track IDs on this device. They do not store audio, file paths, or deck settings.</p>
  </section>;
}
