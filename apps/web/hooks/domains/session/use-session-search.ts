"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import {
  searchSessionMessages,
  searchTaskMessages,
  type MessageSearchHit,
  type SearchMessagesResponse,
} from "@/lib/api/domains/session-api";

type SessionSearchState = {
  isOpen: boolean;
  query: string;
  hits: MessageSearchHit[];
  isSearching: boolean;
  activeHitId: string | null;
};

/** Task-wide search wiring. Omit to keep the legacy single-session behavior. */
export type SessionSearchTaskScope = {
  taskId: string;
  activeSessionId: string | null;
  activateSession: (sessionId: string) => void;
};

const DEBOUNCE_MS = 180;
const MAX_BACKFILL_ITERATIONS = 40;

export type SessionSearchHook = SessionSearchState & {
  open: () => void;
  close: () => void;
  setQuery: (q: string) => void;
  setActiveHit: (id: string | null) => void;
  /** Task-scope keyset pagination. Always false for a session scoped search. */
  hasMore: boolean;
  loadMore: () => void;
  /** True when the search spans the whole task. */
  taskScoped: boolean;
};

type SearchSetters = {
  setHits: Dispatch<SetStateAction<MessageSearchHit[]>>;
  setIsSearching: (v: boolean) => void;
  setHasMore: (v: boolean) => void;
};

/** Debounced fetch + request-ID cancellation for session or task scope. */
function useDebouncedSearch(
  sessionId: string | null | undefined,
  taskScope: SessionSearchTaskScope | undefined,
  setters: SearchSetters,
) {
  const { setHits, setIsSearching, setHasMore } = setters;
  const taskId = taskScope?.taskId;
  const activeSessionId = taskScope?.activeSessionId;
  const requestIdRef = useRef(0);
  const cursorRef = useRef<string | undefined>(undefined);
  const queryRef = useRef("");
  const loadingRef = useRef(false);

  const run = useCallback(
    async (q: string) => {
      const trimmed = q.trim();
      queryRef.current = trimmed;
      cursorRef.current = undefined;
      if (!trimmed) {
        setHits([]);
        setHasMore(false);
        setIsSearching(false);
        return;
      }
      if (!taskId && !sessionId) return;
      const myId = ++requestIdRef.current;
      setIsSearching(true);
      loadingRef.current = true;
      try {
        let resp: SearchMessagesResponse;
        if (taskId) {
          resp = await searchTaskMessages(taskId, activeSessionId, trimmed, { limit: 50 });
        } else if (sessionId) {
          resp = await searchSessionMessages(sessionId, trimmed, 50);
        } else {
          resp = { hits: [], total: 0 };
        }
        if (requestIdRef.current !== myId) return;
        setHits(resp.hits ?? []);
        setHasMore(Boolean(resp.has_more));
        cursorRef.current = resp.next_cursor;
      } catch (err) {
        if (requestIdRef.current !== myId) return;
        console.error("Session search failed:", err);
        setHits([]);
        setHasMore(false);
      } finally {
        loadingRef.current = false;
        if (requestIdRef.current === myId) setIsSearching(false);
      }
    },
    [sessionId, taskId, activeSessionId, setHits, setIsSearching, setHasMore],
  );

  const loadMore = useCallback(async () => {
    if (!taskId || !cursorRef.current || loadingRef.current) return;
    const myId = requestIdRef.current;
    loadingRef.current = true;
    setIsSearching(true);
    try {
      const resp = await searchTaskMessages(taskId, activeSessionId, queryRef.current, {
        limit: 50,
        cursor: cursorRef.current,
      });
      if (requestIdRef.current !== myId) return;
      cursorRef.current = resp.next_cursor;
      setHits((prev) => prev.concat(resp.hits ?? []));
      setHasMore(Boolean(resp.has_more));
    } catch (err) {
      if (requestIdRef.current === myId) console.error("Task search pagination failed:", err);
    } finally {
      loadingRef.current = false;
      if (requestIdRef.current === myId) setIsSearching(false);
    }
  }, [taskId, activeSessionId, setHits, setIsSearching, setHasMore]);

  return { run, loadMore };
}

/** Focus a hit in the DOM with scroll + flash animation. */
function focusMessageElement(id: string, navigate?: (id: string) => HTMLElement | null): boolean {
  const el = navigate ? navigate(id) : document.getElementById(`msg-${id}`);
  if (!el) return false;
  if (!navigate) {
    // Without a navigation callback there is no guard against competing chat scrolling.
    el.scrollIntoView({ block: "center", behavior: "auto" });
  }
  el.classList.remove("search-flash");
  // Force reflow so animation replays when re-clicked
  void el.offsetWidth;
  el.classList.add("search-flash");
  window.setTimeout(() => el.classList.remove("search-flash"), 1400);
  return true;
}

/** setActiveHit + backfill loop with generation-based cancellation. */
function useSetActiveHit(
  loadOlder: (() => Promise<number>) | undefined,
  setActiveHitIdState: (id: string | null) => void,
  genRef: React.RefObject<number>,
  navigate?: (id: string) => HTMLElement | null,
) {
  return useCallback(
    async (id: string | null) => {
      setActiveHitIdState(id);
      if (!id) return;
      const myGen = ++genRef.current;
      if (focusMessageElement(id, navigate)) return;
      if (!loadOlder) return;
      for (let i = 0; i < MAX_BACKFILL_ITERATIONS; i++) {
        const loaded = await loadOlder();
        // Superseded by a newer setActiveHit, or close/unmount bumped genRef.
        if (genRef.current !== myGen) return;
        if (loaded === 0) break;
        if (focusMessageElement(id, navigate)) return;
      }
    },
    [loadOlder, setActiveHitIdState, genRef, navigate],
  );
}

export function useSessionSearch(
  sessionId: string | null | undefined,
  loadOlder?: () => Promise<number>,
  navigate?: (id: string) => HTMLElement | null,
  taskScope?: SessionSearchTaskScope,
): SessionSearchHook {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [hits, setHits] = useState<MessageSearchHit[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [activeHitId, setActiveHitIdState] = useState<string | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Generation counter for setActiveHit — bumping it aborts any in-flight
  // backfill loop (new click, search bar close, or component unmount).
  const activeHitGenRef = useRef(0);
  // Latest hits for cross-session routing inside setActiveHit.
  const hitsRef = useRef<MessageSearchHit[]>([]);
  hitsRef.current = hits;
  // A cross-session hit whose session must activate before the scroll runs.
  const pendingSessionRef = useRef<{ sessionId: string; messageId: string } | null>(null);

  const { run: runSearch, loadMore } = useDebouncedSearch(sessionId, taskScope, {
    setHits,
    setIsSearching,
    setHasMore,
  });

  const setQuery = useCallback(
    (q: string) => {
      setQueryState(q);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = setTimeout(() => runSearch(q), DEBOUNCE_MS);
    },
    [runSearch],
  );

  useEffect(() => {
    const genRef = activeHitGenRef;
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      // Abort any in-flight setActiveHit backfill loop on unmount.
      genRef.current++;
    };
  }, []);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => {
    setIsOpen(false);
    setHits([]);
    setHasMore(false);
    setActiveHitIdState(null);
    setQueryState("");
    setIsSearching(false);
    pendingSessionRef.current = null;
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    // Abort any in-flight setActiveHit backfill loop.
    activeHitGenRef.current++;
  }, []);
  const baseSetActiveHit = useSetActiveHit(
    loadOlder,
    setActiveHitIdState,
    activeHitGenRef,
    navigate,
  );

  const setActiveHit = useCallback(
    (id: string | null) => {
      if (!id) {
        pendingSessionRef.current = null;
        void baseSetActiveHit(null);
        return;
      }
      const hit = hitsRef.current.find((candidate) => candidate.id === id);
      const targetSession = hit?.session_id;
      if (taskScope?.taskId && targetSession && sessionId && targetSession !== sessionId) {
        pendingSessionRef.current = { sessionId: targetSession, messageId: id };
        taskScope.activateSession(targetSession);
        return;
      }
      void baseSetActiveHit(id);
    },
    [baseSetActiveHit, sessionId, taskScope],
  );

  // After a cross-session activation, resume the scroll once the new session
  // is the panel's session. The existing backfill loop handles readiness.
  useEffect(() => {
    const pending = pendingSessionRef.current;
    if (!pending || pending.sessionId !== sessionId) return;
    pendingSessionRef.current = null;
    void baseSetActiveHit(pending.messageId);
  }, [sessionId, baseSetActiveHit]);

  return {
    isOpen,
    query,
    hits,
    isSearching,
    activeHitId,
    hasMore,
    loadMore,
    taskScoped: Boolean(taskScope?.taskId),
    open,
    close,
    setQuery,
    setActiveHit,
  };
}
