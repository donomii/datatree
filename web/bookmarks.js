(function initSavedBookmarksAndRecentsModule() {
  let savedBookmarks = [];
  let savedRecents = [];
  let recentsExpanded = false;
  let bookmarksExpanded = false;
  const pendingRecentKeys = new Set();

  function cleanPathSegments(path) {
    if (!Array.isArray(path)) {
      return [];
    } else {
      return path.map(seg => ({ name: seg.name || '', kind: seg.kind || '' })).filter(seg => seg.name !== '');
    }
  }

  function joinURLPath(parts) {
    return parts.map(part => encodeURIComponent(part)).join('/');
  }

  function deriveURLFromTreePath(path) {
    const clean = cleanPathSegments(path);
    if (clean.length === 0) {
      return '';
    } else if (clean[clean.length - 1].kind === 'url' && /^https?:\/\//i.test(clean[clean.length - 1].name)) {
      return clean[clean.length - 1].name;
    } else {
      const rootInfo = window.datatreeRootInfo || { rootDirectory: '', homeDirectory: '' };
      if (clean[0].name === 'aws' && clean[0].kind === 'aws' && clean.length >= 6) {
        const profile = clean[1].name;
        const regionsEntry = clean[2];
        const regionRaw = clean[3].name;
        const catalogEntry = clean[4];
        const s3Entry = clean[5];
        if (regionsEntry.name === '[regions and resources]' && catalogEntry.name === '[service catalog]' && s3Entry.name === 'S3 buckets') {
          if (clean.length === 6) {
            return '';
          } else {
            const bucket = clean[6].name;
            const tailParts = clean.slice(7).map(seg => seg.name);
            const endsWithSlash = tailParts.length > 0 && tailParts[tailParts.length - 1].endsWith('/');
            const trimmedParts = tailParts.map(p => p.replace(/\/+$/, '')).filter(Boolean);
            let s3URL = `s3://${bucket}`;
            if (trimmedParts.length > 0) {
              s3URL += '/' + joinURLPath(trimmedParts);
              if (endsWithSlash) {
                s3URL += '/';
              } else {
                // Keep exact object path without trailing slash.
              }
            } else {
              // Bucket root S3 URL.
            }
            const regionName = regionRaw.split(' ')[0] || '';
            const params = new URLSearchParams();
            if (profile && profile !== 'default') {
              params.set('profile', profile);
            } else {
              // Default profile uses clean s3://bucket/path form.
            }
            if (regionName) {
              params.set('region', regionName);
            } else {
              // No region annotation present.
            }
            const qs = params.toString();
            return qs ? `${s3URL}?${qs}` : s3URL;
          }
        } else {
          return '';
        }
      } else if (clean[0].name === 'kubernetes' && clean[0].kind === 'kubernetes' && clean.length >= 2) {
        return 'k8s://' + joinURLPath(clean.slice(1).map(seg => seg.name));
      } else if (clean[0].name === 'HOME' && clean[0].kind === 'directory' && rootInfo.homeDirectory) {
        const base = rootInfo.homeDirectory.replace(/\/+$/, '');
        const rel = clean.slice(1).map(seg => seg.name.replace(/\/+$/, ''));
        return 'file://' + base + (rel.length > 0 ? '/' + rel.join('/') : '');
      } else if (clean[0].name === '/' && clean[0].kind === 'directory') {
        const rel = clean.slice(1).map(seg => seg.name.replace(/\/+$/, ''));
        return 'file:///' + rel.join('/');
      } else if (rootInfo.rootDirectory) {
        const base = rootInfo.rootDirectory.replace(/\/+$/, '');
        const rel = clean.map(seg => seg.name.replace(/\/+$/, ''));
        return 'file://' + base + '/' + rel.join('/');
      } else {
        return '';
      }
    }
  }

  function buildSavedRequest(name, resource, path) {
    const clean = cleanPathSegments(path);
    const derivedURL = deriveURLFromTreePath(clean);
    let label = name || '';
    if (derivedURL) {
      label = derivedURL;
    } else if (clean.length > 0) {
      label = clean.map(seg => seg.name).join(' / ');
    } else {
      // Retain caller-supplied name.
    }
    return {
      label: label,
      url: derivedURL,
      path: clean,
      kind: resource ? (resource.kind || '') : (clean.length > 0 ? clean[clean.length - 1].kind : '')
    };
  }

  function savedKeyForItem(item) {
    if (!item) {
      return '';
    } else if (item.url && item.url.trim() !== '') {
      return `url:${item.url.trim()}`;
    } else {
      return `path:${JSON.stringify(cleanPathSegments(item.path))}`;
    }
  }

  function findMatchingBookmark(item) {
    const key = savedKeyForItem(item);
    return savedBookmarks.find(bm => savedKeyForItem(bm) === key) || null;
  }

  function refreshAllRowBookmarkStars() {
    for (const btn of document.querySelectorAll('button.row-bookmark-btn')) {
      const bmKey = btn.dataset.bmKey || '';
      const isSaved = savedBookmarks.some(bm => savedKeyForItem(bm) === bmKey);
      btn.textContent = isSaved ? '★' : '☆';
      btn.title = isSaved ? 'Remove this bookmark' : 'Save persistent bookmark for this location';
    }
  }

  async function openSavedEntry(entry, statusOverride) {
    const status = statusOverride || document.getElementById('bookmark-status') || document.getElementById('url-status');
    if (entry.url && window.isTreeNavURL && window.isTreeNavURL(entry.url)) {
      const urlInput = document.getElementById('url-input');
      if (urlInput) {
        urlInput.value = entry.url;
      } else {
        // URL input unavailable.
      }
      await window.openTreeURL(entry.url, status, '');
      await recordSavedRecent({ label: entry.label, url: entry.url, path: entry.path, kind: entry.kind });
    } else if (entry.url && /^https?:\/\//i.test(entry.url)) {
      const urlInput = document.getElementById('url-input');
      if (urlInput) {
        urlInput.value = entry.url;
      } else {
        // URL input unavailable.
      }
      if (typeof window.openURLAndLoad === 'function') {
        await window.openURLAndLoad();
      } else {
        throw new Error('HTTP URL opener is unavailable');
      }
    } else if (entry.path && entry.path.length > 0 && typeof window.restoreTreePath === 'function') {
      await window.restoreTreePath(entry.path, status);
      await recordSavedRecent({ label: entry.label, url: entry.url, path: entry.path, kind: entry.kind });
    } else {
      throw new Error(`Bookmark "${entry.label}" has no navigable URL or path`);
    }
  }

  function renderSavedChildren(container, entries, branchType, headerName) {
    container.innerHTML = '';
    const header = document.createElement('div');
    header.style.display = 'flex';
    header.style.alignItems = 'center';
    header.style.gap = '0.5rem';
    const titleSpan = document.createElement('span');
    titleSpan.textContent = `${headerName} (${entries.length} ${entries.length === 1 ? 'item' : 'items'})`;
    header.appendChild(titleSpan);
    if (branchType === 'recents' && entries.length > 0) {
      const clearBtn = document.createElement('button');
      clearBtn.type = 'button';
      clearBtn.textContent = 'Clear recents';
      clearBtn.title = 'Clear saved recent items history';
      clearBtn.onclick = async event => {
        event.stopPropagation();
        await clearAllRecents();
      };
      header.appendChild(clearBtn);
    } else {
      // No clear button needed when recents is empty or displaying bookmarks.
    }
    container.appendChild(header);

    if (entries.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'meta';
      empty.textContent = branchType === 'bookmarks'
        ? 'No saved bookmarks yet. Click ☆ beside a row or URL to bookmark it.'
        : 'No recent items recorded yet. Open a URL or expand items in the tree.';
      container.appendChild(empty);
      return [];
    } else {
      const contentDiv = document.createElement('div');
      const renderedChildren = [];
      entries.forEach(entry => {
        const row = document.createElement('div');
        row.className = 'file-item';
        const expanderSlot = document.createElement('span');
        expanderSlot.className = 'expander';
        const openBtn = document.createElement('button');
        openBtn.type = 'button';
        openBtn.textContent = '↗';
        openBtn.title = `Open ${entry.label} in the tree`;
        openBtn.onclick = async event => {
          event.stopPropagation();
          openBtn.disabled = true;
          try {
            await openSavedEntry(entry);
          } catch (err) {
            const status = document.getElementById('bookmark-status');
            if (status) {
              status.className = 'error';
              status.textContent = `Could not open "${entry.label}": ${err.message}`;
            } else {
              // Status element absent.
            }
          } finally {
            openBtn.disabled = false;
          }
        };
        expanderSlot.appendChild(openBtn);

        const recSlot = document.createElement('span');
        recSlot.className = 'recursive-expander';
        const bmSlot = document.createElement('span');
        bmSlot.className = 'bookmark';
        const iconSpan = document.createElement('span');
        iconSpan.textContent = branchType === 'bookmarks' ? '★' : '🕘';

        const nameSpan = document.createElement('span');
        const code = document.createElement('code');
        code.textContent = entry.label;
        const meta = document.createElement('span');
        meta.className = 'meta';
        const targetSummary = entry.url || (entry.path ? entry.path.map(s => s.name).join(' / ') : '');
        const parts = [];
        if (entry.kind) {
          parts.push(entry.kind);
        } else {
          // Omit empty kind.
        }
        if (targetSummary && targetSummary !== entry.label) {
          parts.push(targetSummary);
        } else {
          // Avoid repeating identical label and URL.
        }
        meta.textContent = parts.length > 0 ? ` (${parts.join(', ')})` : '';
        nameSpan.appendChild(code);
        nameSpan.appendChild(meta);

        row.appendChild(expanderSlot);
        row.appendChild(recSlot);
        row.appendChild(bmSlot);
        row.appendChild(iconSpan);
        row.appendChild(nameSpan);

        if (branchType === 'recents') {
          const starBtn = document.createElement('button');
          starBtn.type = 'button';
          starBtn.className = 'row-bookmark-btn';
          const reqItem = { label: entry.label, url: entry.url, path: entry.path, kind: entry.kind };
          starBtn.dataset.bmKey = savedKeyForItem(reqItem);
          const already = findMatchingBookmark(reqItem) !== null;
          starBtn.textContent = already ? '★' : '☆';
          starBtn.title = already ? 'Remove from bookmarks' : 'Save to bookmarks';
          starBtn.onclick = async event => {
            event.stopPropagation();
            await toggleSavedBookmark(reqItem);
          };
          bmSlot.appendChild(starBtn);
        } else {
          const removeBtn = document.createElement('button');
          removeBtn.type = 'button';
          removeBtn.textContent = 'Remove';
          removeBtn.title = `Delete bookmark "${entry.label}"`;
          removeBtn.onclick = async event => {
            event.stopPropagation();
            await removeBookmarkByID(entry.id);
          };
          row.appendChild(removeBtn);
        }

        contentDiv.appendChild(row);
        renderedChildren.push({
          item: row,
          resource: { name: `${entry.label} ${entry.url || ''}`.trim(), kind: entry.kind || branchType, size: -1 },
          handle: '',
          name: entry.label,
          path: null
        });
      });
      container.appendChild(contentDiv);
      return renderedChildren;
    }
  }

  function renderSavedBranches() {
    const container = document.getElementById('saved-tree');
    if (!container) {
      return;
    } else {
      container.innerHTML = '';
      const navRendered = [];
      const specs = [
        { key: 'recents', name: 'recents', icon: '🕘', entries: savedRecents, isExpanded: () => recentsExpanded, setExpanded: v => { recentsExpanded = v; } },
        { key: 'bookmarks', name: 'bookmarks', icon: '★', entries: savedBookmarks, isExpanded: () => bookmarksExpanded, setExpanded: v => { bookmarksExpanded = v; } }
      ];

      specs.forEach(spec => {
        const item = document.createElement('div');
        item.className = `file-item kind-${spec.key}`;
        const expanderSlot = document.createElement('span');
        expanderSlot.className = 'expander';
        const toggleBtn = document.createElement('button');
        toggleBtn.type = 'button';
        expanderSlot.appendChild(toggleBtn);

        const recSlot = document.createElement('span');
        recSlot.className = 'recursive-expander';
        const bmSlot = document.createElement('span');
        bmSlot.className = 'bookmark';
        const iconSpan = document.createElement('span');
        iconSpan.textContent = spec.icon;

        const nameSpan = document.createElement('span');
        const code = document.createElement('code');
        code.textContent = spec.name;
        const meta = document.createElement('span');
        meta.className = 'meta';
        meta.textContent = ` (branch, ${spec.entries.length} ${spec.entries.length === 1 ? 'item' : 'items'})`;
        nameSpan.appendChild(code);
        nameSpan.appendChild(meta);

        item.appendChild(expanderSlot);
        item.appendChild(recSlot);
        item.appendChild(bmSlot);
        item.appendChild(iconSpan);
        item.appendChild(nameSpan);
        container.appendChild(item);

        const subTree = document.createElement('div');
        subTree.className = 'tree';
        subTree.style.marginTop = '0.5rem';
        subTree.foldByHiding = true;
        if (typeof window.rememberBranch === 'function') {
          window.rememberBranch(subTree, item, spec.name);
        } else {
          subTree.branchOwner = item;
        }
        const childRows = renderSavedChildren(subTree, spec.entries, spec.key, spec.name);
        subTree.renderedResources = childRows;
        subTree.hidden = !spec.isExpanded();
        container.appendChild(subTree);

        function syncArrow() {
          const open = !subTree.hidden;
          spec.setExpanded(open);
          toggleBtn.textContent = open ? '▾' : '▸';
          toggleBtn.title = open ? `Collapse ${spec.name}` : `Expand ${spec.name}`;
        }
        syncArrow();

        toggleBtn.onclick = event => {
          event.stopPropagation();
          subTree.hidden = !subTree.hidden;
          syncArrow();
        };

        navRendered.push({
          item: item,
          resource: { name: spec.name, kind: spec.key, size: spec.entries.length },
          handle: '',
          name: spec.name,
          path: null
        });
      });

      window.savedNavRenderedResources = navRendered;
      refreshAllRowBookmarkStars();
    }
  }

  async function refreshBookmarksAndRecents() {
    try {
      const [bmResp, recResp] = await Promise.all([
        fetch('/api/bookmarks'),
        fetch('/api/recents')
      ]);
      if (bmResp.ok) {
        const data = await bmResp.json();
        savedBookmarks = data.entries || [];
      } else {
        // Preserve previous bookmarks state on transient read error.
      }
      if (recResp.ok) {
        const data = await recResp.json();
        savedRecents = data.entries || [];
      } else {
        // Preserve previous recents state on transient read error.
      }
      renderSavedBranches();
    } catch (_) {
      // Non-fatal if browser is in direct file:// mode or still starting up.
    }
  }

  async function toggleSavedBookmark(reqItem) {
    const status = document.getElementById('bookmark-status');
    const existing = findMatchingBookmark(reqItem);
    try {
      if (existing) {
        const resp = await fetch(`/api/bookmarks?id=${encodeURIComponent(existing.id)}`, { method: 'DELETE' });
        if (!resp.ok) {
          throw new Error(await resp.text());
        } else {
          const data = await resp.json();
          savedBookmarks = data.entries || [];
          renderSavedBranches();
          if (status) {
            status.className = 'meta';
            status.textContent = `Removed bookmark for ${reqItem.label}.`;
          } else {
            // Status readout absent.
          }
          return false;
        }
      } else {
        const resp = await fetch('/api/bookmarks', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(reqItem)
        });
        if (!resp.ok) {
          throw new Error(await resp.text());
        } else {
          const data = await resp.json();
          savedBookmarks = data.entries || [];
          bookmarksExpanded = true;
          renderSavedBranches();
          if (status) {
            status.className = 'meta';
            status.textContent = `Bookmarked ${reqItem.label}.`;
          } else {
            // Status readout absent.
          }
          return true;
        }
      }
    } catch (error) {
      if (status) {
        status.className = 'error';
        status.textContent = `Could not update bookmark: ${error.message}`;
      } else {
        // Status readout absent.
      }
      return false;
    }
  }

  async function removeBookmarkByID(id) {
    const status = document.getElementById('bookmark-status');
    try {
      const resp = await fetch(`/api/bookmarks?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
      if (!resp.ok) {
        throw new Error(await resp.text());
      } else {
        const data = await resp.json();
        savedBookmarks = data.entries || [];
        renderSavedBranches();
        if (status) {
          status.className = 'meta';
          status.textContent = 'Bookmark removed.';
        } else {
          // Status readout absent.
        }
      }
    } catch (error) {
      if (status) {
        status.className = 'error';
        status.textContent = `Could not remove bookmark: ${error.message}`;
      } else {
        // Status readout absent.
      }
    }
  }

  async function recordSavedRecent(reqItem) {
    if (!reqItem || (!reqItem.url && (!reqItem.path || reqItem.path.length === 0))) {
      return;
    } else {
      const key = savedKeyForItem(reqItem);
      if (pendingRecentKeys.has(key)) {
        return;
      } else {
        pendingRecentKeys.add(key);
        try {
          const resp = await fetch('/api/recents', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(reqItem)
          });
          if (resp.ok) {
            const data = await resp.json();
            savedRecents = data.entries || [];
            renderSavedBranches();
          } else {
            // Ignore non-fatal recent history write errors.
          }
        } catch (_) {
          // Recent tracking is best-effort.
        } finally {
          pendingRecentKeys.delete(key);
        }
      }
    }
  }

  async function clearAllRecents() {
    const status = document.getElementById('bookmark-status');
    try {
      const resp = await fetch('/api/recents', { method: 'DELETE' });
      if (!resp.ok) {
        throw new Error(await resp.text());
      } else {
        const data = await resp.json();
        savedRecents = data.entries || [];
        renderSavedBranches();
        if (status) {
          status.className = 'meta';
          status.textContent = 'Cleared recent items.';
        } else {
          // Status readout absent.
        }
      }
    } catch (error) {
      if (status) {
        status.className = 'error';
        status.textContent = `Could not clear recents: ${error.message}`;
      } else {
        // Status readout absent.
      }
    }
  }

  window.deriveSavedEntry = buildSavedRequest;
  window.savedEntryKeyForRow = function savedEntryKeyForRow(name, resource, path) {
    return savedKeyForItem(buildSavedRequest(name, resource, path));
  };
  window.isRowBookmarked = function isRowBookmarked(name, resource, path) {
    return findMatchingBookmark(buildSavedRequest(name, resource, path)) !== null;
  };
  window.toggleRowBookmark = async function toggleRowBookmark(name, resource, path) {
    return toggleSavedBookmark(buildSavedRequest(name, resource, path));
  };
  window.bookmarkCurrentInputURL = async function bookmarkCurrentInputURL() {
    const urlInput = document.getElementById('url-input');
    const urlStatus = document.getElementById('url-status');
    const rawURL = urlInput ? urlInput.value.trim() : '';
    if (!rawURL) {
      if (urlStatus) {
        urlStatus.className = 'error';
        urlStatus.textContent = 'Enter a URL above first to bookmark it';
      } else {
        // Status element unavailable.
      }
      return;
    } else {
      await toggleSavedBookmark({ label: rawURL, url: rawURL, kind: 'url' });
    }
  };
  window.recordRecentForRow = async function recordRecentForRow(name, resource, path) {
    if (!name || name === '[next page]' || !path || path.length === 0) {
      return;
    } else if (resource && (resource.kind === 'kv' || resource.kind === 'error')) {
      return;
    } else {
      await recordSavedRecent(buildSavedRequest(name, resource, path));
    }
  };
  window.recordRecentForURL = async function recordRecentForURL(rawURL, kind) {
    if (!rawURL) {
      return;
    } else {
      await recordSavedRecent({ label: rawURL, url: rawURL, kind: kind || 'url' });
    }
  };
  window.initBookmarksAndRecents = refreshBookmarksAndRecents;
  window.renderSavedBranches = renderSavedBranches;
})();
