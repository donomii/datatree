function snapshotBranchForSearch(tree) {
  const rows = (tree.renderedResources || []).map(entry => {
    const nestedTree = entry.item?.nextElementSibling;
    let sub = entry.item?.cachedBranch || null;
    if (nestedTree?.classList.contains('tree') && nestedTree.branchOwner === entry.item && !nestedTree.dataset.searchTemporary && nestedTree.renderedResources) {
      sub = snapshotBranchForSearch(nestedTree);
    } else {
      // Preserve any previously cached folded branch on this child item.
    }
    return {
      resource: entry.resource,
      handle: entry.handle,
      name: entry.name,
      path: entry.path,
      cachedBranch: sub
    };
  });
  return {
    headerText: tree.firstElementChild?.textContent || '',
    parentPath: tree.parentBranchPath ?? null,
    rows: rows
  };
}

function cacheBranchForSearch(item, tree) {
  if (!tree || tree.dataset.searchTemporary === '1' || !tree.renderedResources) {
    return;
  } else {
    item.cachedBranch = snapshotBranchForSearch(tree);
  }
}

function tokenizeSearchQuery(query) {
  return query.trim().toLowerCase().split(/\s+/).filter(Boolean);
}

function matchesSearchTokens(name, kind, tokens) {
  if (tokens.length === 0) {
    return true;
  } else {
    const haystack = `${name || ''} ${kind || ''}`.toLowerCase();
    return tokens.every(token => haystack.includes(token));
  }
}

function branchHasMatch(branch, tokens) {
  if (!branch || !branch.rows) {
    return false;
  } else {
    return branch.rows.some(entry => {
      if (matchesSearchTokens(entry.name, entry.resource.kind, tokens)) {
        return true;
      } else {
        return branchHasMatch(entry.cachedBranch, tokens);
      }
    });
  }
}

function renderCachedBranchForSearch(item, cachedBranch) {
  const subContainer = document.createElement('div');
  subContainer.className = 'tree';
  subContainer.dataset.searchTemporary = '1';
  const branchName = item.querySelector('code')?.textContent || '';
  rememberBranch(subContainer, item, branchName);
  subContainer.style.marginTop = '0.5rem';
  const header = document.createElement('div');
  header.textContent = cachedBranch.headerText || `${branchName} (cached folded branch)`;
  subContainer.appendChild(header);
  const contentDiv = document.createElement('div');
  const resources = cachedBranch.rows.map(row => row.resource);
  const rendered = renderDirectory(contentDiv, resources, cachedBranch.parentPath);
  for (let i = 0; i < rendered.length; i++) {
    if (cachedBranch.rows[i]) {
      rendered[i].path = cachedBranch.rows[i].path;
      rendered[i].item.cachedBranch = cachedBranch.rows[i].cachedBranch;
    } else {
      // No cached row metadata for this index.
    }
  }
  subContainer.renderedResources = rendered;
  subContainer.parentBranchPath = cachedBranch.parentPath;
  subContainer.appendChild(contentDiv);
  item.parentNode.insertBefore(subContainer, item.nextSibling);
  setExpansionState(item, true);
  return subContainer;
}

function restoreSpeedsearchTree() {
  for (const tempTree of document.querySelectorAll('.tree[data-search-temporary="1"]')) {
    const owner = tempTree.branchOwner;
    tempTree.remove();
    if (owner && owner.isConnected) {
      setExpansionState(owner, false);
    } else {
      // Disconnected owner needs no arrow update.
    }
  }
  for (const revealed of document.querySelectorAll('.tree[data-search-revealed="1"]')) {
    revealed.hidden = true;
    delete revealed.dataset.searchRevealed;
    if (revealed.branchOwner) {
      setExpansionState(revealed.branchOwner, false);
    } else {
      // No owner row to update.
    }
  }
  for (const el of document.querySelectorAll('#tree .file-item, #tree .tree, #upload-tree .file-item, #upload-tree > .tree, #upload-tree .tree')) {
    if (!el.foldByHiding || el.dataset.searchRevealed !== undefined) {
      el.hidden = false;
    } else {
      // Keep user-folded upload roots hidden unless revealed by search.
    }
  }
  const uploadTree = document.getElementById('upload-tree');
  if (uploadTree) {
    for (const sub of uploadTree.children) {
      sub.hidden = false;
      for (const item of sub.querySelectorAll('.file-item')) {
        item.hidden = false;
      }
    }
  } else {
    // Upload container absent.
  }
  if (typeof window.renderSavedBranches === 'function') {
    window.renderSavedBranches();
  } else {
    // Saved bookmarks/recents module not loaded.
  }
  const resultsBox = document.getElementById('speedsearch-results');
  if (resultsBox) {
    resultsBox.hidden = true;
    resultsBox.innerHTML = '';
  } else {
    // Results container absent.
  }
}

function filterRenderedList(rendered, tokens, visibleHandles) {
  let matchCount = 0;
  for (const entry of rendered || []) {
    const selfMatch = matchesSearchTokens(entry.name, entry.resource.kind, tokens);
    let childMatches = 0;
    let childTree = entry.item.nextElementSibling;
    if (childTree && childTree.classList.contains('tree') && childTree.branchOwner === entry.item) {
      if (childTree.foldByHiding && childTree.hidden) {
        if (branchHasMatch({rows: childTree.renderedResources || []}, tokens) || hasMatchingDescendantDOM(childTree, tokens)) {
          childTree.hidden = false;
          childTree.dataset.searchRevealed = '1';
          setExpansionState(entry.item, true);
        } else {
          // Keep hidden folded branch closed when it has no matches.
        }
      } else {
        // Open child tree stays open during filtering.
      }
      if (!childTree.hidden) {
        if (childTree.renderedResources) {
          childMatches = filterRenderedList(childTree.renderedResources, tokens, visibleHandles);
        } else {
          // Branch without child resource rows (e.g. preview-only node).
        }
      } else {
        // Hidden subtree has no visible matches.
      }
    } else if (entry.item.cachedBranch && branchHasMatch(entry.item.cachedBranch, tokens)) {
      childTree = renderCachedBranchForSearch(entry.item, entry.item.cachedBranch);
      childMatches = filterRenderedList(childTree.renderedResources, tokens, visibleHandles);
    } else {
      // Unopened leaf or folded branch with no cached matches.
    }
    const rowVisible = selfMatch || childMatches > 0;
    entry.item.hidden = !rowVisible;
    if (childTree && childTree.classList.contains('tree') && childTree.branchOwner === entry.item) {
      if (!rowVisible || (selfMatch && childMatches === 0 && !childTree.dataset.searchTemporary)) {
        if (childMatches === 0) {
          childTree.hidden = true;
          if (!childTree.foldByHiding) {
            childTree.dataset.searchRevealed = '1';
          } else {
            // Preserve foldByHiding flag.
          }
        } else {
          childTree.hidden = false;
        }
      } else {
        childTree.hidden = false;
      }
    } else {
      // No child tree to toggle.
    }
    if (selfMatch) {
      matchCount += 1;
      if (entry.handle) {
        visibleHandles.add(entry.handle);
      } else {
        // Handleless item still counted.
      }
    } else {
      // Ancestor revealed solely for matching descendants.
    }
    matchCount += childMatches;
  }
  return matchCount;
}

function hasMatchingDescendantDOM(container, tokens) {
  for (const item of container.querySelectorAll('.file-item')) {
    const label = item.querySelector('code')?.textContent || '';
    const meta = item.querySelector('.meta')?.textContent || '';
    if (matchesSearchTokens(label, meta, tokens)) {
      return true;
    } else {
      // Keep scanning descendants.
    }
  }
  return false;
}

function collectUploadSearchHandles() {
  const handles = [];
  const uploadTree = document.getElementById('upload-tree');
  if (!uploadTree) {
    return handles;
  } else {
    for (const sub of uploadTree.children) {
      for (const tree of sub.querySelectorAll('.tree')) {
        for (const entry of tree.renderedResources || []) {
          if (entry.handle && isExpandable(entry.resource.kind)) {
            handles.push(entry.handle);
          } else {
            // Non-expandable upload entry skipped.
          }
        }
      }
    }
    return handles;
  }
}

function applyClientSpeedsearch(tokens) {
  const visibleHandles = new Set();
  let localMatches = 0;
  const uploadTree = document.getElementById('upload-tree');
  if (uploadTree) {
    for (const sub of uploadTree.children) {
      const uploadRow = sub.querySelector(':scope > .file-item');
      const uploadContent = uploadRow ? uploadRow.nextElementSibling : null;
      const uploadName = uploadRow?.querySelector('code')?.textContent || '';
      const selfMatch = matchesSearchTokens(uploadName, 'upload', tokens);
      let childMatches = 0;
      if (uploadContent && uploadContent.classList.contains('tree')) {
        const treeContents = uploadContent.lastElementChild;
        const rendered = treeContents?.renderedResources || [];
        const wasHidden = uploadContent.hidden;
        childMatches = filterRenderedList(rendered, tokens, visibleHandles);
        if (wasHidden && childMatches > 0) {
          uploadContent.hidden = false;
          uploadContent.dataset.searchRevealed = '1';
          setExpansionState(uploadRow, true);
        } else if (wasHidden) {
          uploadContent.hidden = true;
        } else {
          // Leave previously open upload content open.
        }
      } else {
        // Upload container has no content branch.
      }
      if (selfMatch || childMatches > 0) {
        sub.hidden = false;
        if (uploadRow) {
          uploadRow.hidden = false;
        } else {
          // No upload row present.
        }
        localMatches += (selfMatch ? 1 : 0) + childMatches;
      } else {
        sub.hidden = true;
      }
    }
  } else {
    // No upload tree container.
  }
  localMatches += filterRenderedList(window.savedNavRenderedResources || [], tokens, visibleHandles);
  localMatches += filterRenderedList(window.rootRenderedResources || [], tokens, visibleHandles);
  return { localMatches, visibleHandles };
}

function renderDeepSearchMatches(matches, visibleHandles) {
  const resultsBox = document.getElementById('speedsearch-results');
  if (!resultsBox) {
    return 0;
  } else {
    resultsBox.innerHTML = '';
    const extra = (matches || []).filter(m => !m.resource?.handle || !visibleHandles.has(m.resource.handle));
    if (extra.length === 0) {
      resultsBox.hidden = true;
      return 0;
    } else {
      const heading = document.createElement('div');
      heading.className = 'meta';
      heading.textContent = `Matches inside folded / unopened branches (${extra.length}):`;
      resultsBox.appendChild(heading);
      for (const match of extra) {
        const wrapper = document.createElement('div');
        wrapper.style.marginTop = '0.35rem';
        if (match.path && match.path.length > 0) {
          const crumb = document.createElement('div');
          crumb.className = 'meta';
          crumb.textContent = match.path.map(seg => seg.name).join(' ▸ ');
          wrapper.appendChild(crumb);
        } else {
          // Root-level match needs no breadcrumb.
        }
        const rowContainer = document.createElement('div');
        const rowPath = (match.path || []).map(seg => ({name: seg.name, kind: seg.kind}));
        renderDirectory(rowContainer, [match.resource], rowPath);
        wrapper.appendChild(rowContainer);
        resultsBox.appendChild(wrapper);
      }
      resultsBox.hidden = false;
      return extra.length;
    }
  }
}

document.addEventListener('DOMContentLoaded', () => {
  const input = document.getElementById('speedsearch-input');
  const clearButton = document.getElementById('speedsearch-clear');
  const status = document.getElementById('speedsearch-status');
  if (!input || !clearButton || !status) {
    return;
  } else {
    let timerId = null;
    let activeController = null;

    function clearSearch() {
      if (timerId !== null) {
        clearTimeout(timerId);
        timerId = null;
      } else {
        // No pending search timer.
      }
      if (activeController !== null) {
        activeController.abort();
        activeController = null;
      } else {
        // No active search request.
      }
      input.value = '';
      clearButton.hidden = true;
      status.className = 'meta';
      status.textContent = '';
      restoreSpeedsearchTree();
    }

    async function executeSearch() {
      const query = input.value;
      const tokens = tokenizeSearchQuery(query);
      if (tokens.length === 0) {
        clearSearch();
        return;
      } else {
        clearButton.hidden = false;
        restoreSpeedsearchTree();
        const { localMatches, visibleHandles } = applyClientSpeedsearch(tokens);
        status.className = 'meta';
        status.textContent = `${localMatches} visible/cached match(es); searching folded branches...`;

        if (timerId !== null) {
          clearTimeout(timerId);
        } else {
          // No previous debounce timer.
        }
        if (activeController !== null) {
          activeController.abort();
        } else {
          // No previous fetch request.
        }
        timerId = setTimeout(async () => {
          timerId = null;
          const controller = new AbortController();
          activeController = controller;
          try {
            const response = await fetch('/api/search', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({
                query: query,
                extra_handles: collectUploadSearchHandles()
              }),
              signal: controller.signal
            });
            if (!response.ok) {
              throw new Error(await response.text());
            } else {
              const data = await response.json();
              if (controller.signal.aborted || input.value !== query) {
                return;
              } else {
                const deepCount = renderDeepSearchMatches(data.matches, visibleHandles);
                const total = localMatches + deepCount;
                const limitNote = data.truncated ? '; search limit reached' : '';
                status.className = 'meta';
                status.textContent = `Speedsearch found ${total} match(es) (${localMatches} in loaded branches, ${deepCount} in folded branches; scanned ${data.scanned} items${limitNote}).`;
              }
            }
          } catch (error) {
            if (error.name === 'AbortError') {
              return;
            } else {
              status.className = 'error';
              status.textContent = `Speedsearch error: ${error.message}`;
            }
          } finally {
            if (activeController === controller) {
              activeController = null;
            } else {
              // Newer controller already replaced this one.
            }
          }
        }, 150);
      }
    }

    input.addEventListener('input', executeSearch);
    clearButton.addEventListener('click', clearSearch);
    input.addEventListener('keydown', event => {
      if (event.key === 'Escape') {
        clearSearch();
      } else {
        // Non-Escape keys continue normal input handling.
      }
    });
  }
});
