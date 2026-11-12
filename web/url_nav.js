function normalizeLocalPath(rawPath) {
  if (!rawPath) {
    return '';
  } else {
    const collapsed = rawPath.replace(/\/+/g, '/');
    if (collapsed.length > 1 && collapsed.endsWith('/')) {
      return collapsed.slice(0, -1);
    } else {
      return collapsed;
    }
  }
}

function parseNavigableURL(rawInput) {
  const trimmed = (rawInput || '').trim();
  if (!trimmed) {
    return null;
  } else if (trimmed.startsWith('/')) {
    return {
      scheme: 'file',
      raw: trimmed,
      path: normalizeLocalPath(trimmed),
      trailingSlash: trimmed.endsWith('/') && trimmed.length > 1
    };
  } else {
    const schemeMatch = trimmed.match(/^([a-zA-Z][a-zA-Z0-9+.-]*):\/\/(.*)$/);
    if (!schemeMatch) {
      return null;
    } else {
      const scheme = schemeMatch[1].toLowerCase();
      if (scheme === 'http' || scheme === 'https') {
        return null;
      } else if (scheme === 'file') {
        try {
          const parsed = new URL(trimmed);
          const decodedPath = decodeURIComponent(parsed.pathname || '/');
          return {
            scheme: 'file',
            raw: trimmed,
            path: normalizeLocalPath(decodedPath),
            trailingSlash: decodedPath.endsWith('/') && decodedPath.length > 1
          };
        } catch (error) {
          return { scheme: 'invalid', error: `Invalid file:// URL: ${error.message}` };
        }
      } else if (scheme === 's3') {
        const rest = schemeMatch[2];
        const querySplit = rest.split('?');
        const mainPart = querySplit[0];
        const query = new URLSearchParams(querySplit.slice(1).join('?'));
        const hashSplit = mainPart.split('#')[0];
        const slashIndex = hashSplit.indexOf('/');
        const authority = slashIndex >= 0 ? hashSplit.slice(0, slashIndex) : hashSplit;
        const keyPart = slashIndex >= 0 ? hashSplit.slice(slashIndex + 1) : '';
        let profile = query.get('profile') || '';
        let bucket = authority;
        const atIndex = authority.indexOf('@');
        if (atIndex >= 0) {
          if (!profile) {
            profile = decodeURIComponent(authority.slice(0, atIndex));
          } else {
            // Query parameter profile takes precedence over userinfo prefix.
          }
          bucket = authority.slice(atIndex + 1);
        } else {
          // No profile userinfo supplied in authority.
        }
        bucket = decodeURIComponent(bucket.trim());
        const decodedKey = keyPart.split('/').map(seg => {
          try {
            return decodeURIComponent(seg);
          } catch (_) {
            return seg;
          }
        });
        const trailingSlash = keyPart.length > 0 && keyPart.endsWith('/');
        const segments = decodedKey.filter(seg => seg.length > 0);
        if (!bucket) {
          return { scheme: 'invalid', error: `Expected an S3 bucket name in "${trimmed}" (e.g. s3://bucket/path/to/file)` };
        } else {
          return {
            scheme: 's3',
            raw: trimmed,
            profile: profile,
            region: query.get('region') || '',
            bucket: bucket,
            segments: segments,
            trailingSlash: trailingSlash
          };
        }
      } else if (scheme === 'k8s') {
        const rest = schemeMatch[2].split('#')[0].split('?')[0];
        const segments = rest.split('/').map(seg => {
          try {
            return decodeURIComponent(seg);
          } catch (_) {
            return seg;
          }
        }).filter(seg => seg.length > 0);
        if (segments.length === 0) {
          return { scheme: 'invalid', error: `Expected a Kubernetes context in "${trimmed}" (e.g. k8s://context/namespace/pods/name)` };
        } else {
          return {
            scheme: 'k8s',
            raw: trimmed,
            segments: segments,
            trailingSlash: rest.endsWith('/')
          };
        }
      } else {
        return null;
      }
    }
  }
}

window.isTreeNavURL = function isTreeNavURL(rawInput) {
  const parsed = parseNavigableURL(rawInput);
  return parsed !== null;
};

window.restoreTreePath = async function restoreTreePath(path, statusElement) {
  const status = statusElement || document.getElementById('bookmark-status');
  const rootEntries = window.rootRenderedResources || [];
  if (!Array.isArray(path) || path.length === 0) {
    throw new Error('Bookmark path is empty');
  } else {
    let current = rootEntries;
    for (let index = 0; index < path.length; index++) {
      const segment = path[index];
      const target = await findChildAcrossPages(
        current,
        candidate => candidate.name === segment.name && (!segment.kind || candidate.resource.kind === segment.kind),
        status,
        segment.name
      );
      if (!target || !target.handle) {
        throw new Error(`Could not find ${segment.name} (${segment.kind || 'unknown'}) in the current tree`);
      } else if (index === path.length - 1) {
        if (isExpandable(target.resource.kind) && target.resource.kind !== 'url') {
          await ensureNodeExpanded(target);
        } else {
          // Leaf node highlighted without expansion.
        }
        highlightTargetRow(target);
        if (status) {
          status.className = 'meta';
          status.textContent = `Opened ${segment.name}.`;
        } else {
          // Status readout optional.
        }
        return target;
      } else if (!isExpandable(target.resource.kind)) {
        throw new Error(`"${segment.name}" (${target.resource.kind}) cannot be expanded to reach descendants`);
      } else {
        current = await ensureNodeExpanded(target);
      }
    }
    return null;
  }
};

function clearTargetHighlights() {
  for (const el of document.querySelectorAll('.url-target-highlight')) {
    el.classList.remove('url-target-highlight');
  }
}

function highlightTargetRow(entry) {
  clearTargetHighlights();
  if (entry && entry.item) {
    entry.item.classList.add('url-target-highlight');
    entry.item.scrollIntoView({ behavior: 'smooth', block: 'center' });
  } else {
    // No DOM row available to scroll into view.
  }
}

async function ensureNodeExpanded(entry, selectedDriver = null) {
  if (!entry || !entry.item || !entry.handle) {
    throw new Error(`Cannot expand "${entry ? entry.name : 'unknown'}": missing runtime handle`);
  } else {
    const existing = entry.item.nextSibling;
    if (existing && existing.className === 'tree' && selectedDriver === null) {
      const hasError = existing.firstElementChild && existing.firstElementChild.classList.contains('error');
      if (!hasError) {
        return existing.renderedResources || [];
      } else {
        await expandResource(entry.item, entry.resource, entry.handle, entry.name, entry.path);
      }
    } else {
      // Expand node from collapsed state or with explicit filter.
    }
    await expandResource(entry.item, entry.resource, entry.handle, entry.name, entry.path, false, selectedDriver);
    const subTree = entry.item.nextSibling;
    if (!subTree || subTree.className !== 'tree') {
      return [];
    } else {
      const header = subTree.firstElementChild;
      if (header && header.classList.contains('error')) {
        throw new Error(header.textContent);
      } else {
        return subTree.renderedResources || [];
      }
    }
  }
}

async function findSegmentAcrossPages(entries, seg, isLast, trailingSlash, remainingJoined, status, label, maxPages = 20) {
  let currentEntries = entries || [];
  for (let page = 1; page <= maxPages; page++) {
    const resolved = findMatchingSegment(currentEntries, seg, isLast, trailingSlash, remainingJoined);
    if (resolved) {
      return resolved;
    } else {
      const nextPage = currentEntries.find(candidate => candidate.name === '[next page]' && candidate.handle && isExpandable(candidate.resource.kind));
      if (!nextPage) {
        return null;
      } else {
        if (status) {
          status.textContent = `Searching ${label} (page ${page + 1})...`;
        } else {
          // Status element optional.
        }
        currentEntries = await ensureNodeExpanded(nextPage);
      }
    }
  }
  return null;
}

async function findChildAcrossPages(entries, matcher, status, label, maxPages = 20) {
  let currentEntries = entries || [];
  for (let page = 1; page <= maxPages; page++) {
    const directMatch = currentEntries.find(matcher);
    if (directMatch) {
      return directMatch;
    } else {
      const nextPage = currentEntries.find(candidate => candidate.name === '[next page]' && candidate.handle && isExpandable(candidate.resource.kind));
      if (!nextPage) {
        return null;
      } else {
        if (status) {
          status.textContent = `Searching ${label} (page ${page + 1})...`;
        } else {
          // Status element optional.
        }
        currentEntries = await ensureNodeExpanded(nextPage);
      }
    }
  }
  return null;
}

function findMatchingSegment(entries, seg, isLast, trailingSlash, remainingJoined) {
  if (!entries || entries.length === 0) {
    return null;
  } else {
    if (!isLast || trailingSlash) {
      const dirMatch = entries.find(e => e.name === `${seg}/`);
      if (dirMatch) {
        return { entry: dirMatch, consumedAll: false };
      } else {
        // Fall through to exact segment match (directories or containers).
      }
    } else {
      // Final non-slash target prefers exact object/file match first.
    }
    const exactMatch = entries.find(e => e.name === seg);
    if (exactMatch) {
      return { entry: exactMatch, consumedAll: false };
    } else {
      const slashFallback = entries.find(e => e.name === `${seg}/`);
      if (slashFallback) {
        return { entry: slashFallback, consumedAll: false };
      } else if (remainingJoined && remainingJoined !== seg) {
        const flatMatch = entries.find(e => e.name === remainingJoined) || entries.find(e => e.name === `${remainingJoined}/`);
        if (flatMatch) {
          return { entry: flatMatch, consumedAll: true };
        } else {
          return null;
        }
      } else {
        return null;
      }
    }
  }
}

async function walkTreeSegments(startChildren, segments, trailingSlash, status, contextLabel, selectedDriver) {
  if (segments.length === 0) {
    return null;
  } else {
    let children = startChildren;
    let currentEntry = null;
    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i];
      const isLast = i === segments.length - 1;
      const remainingJoined = segments.slice(i).join('/');
      if (status) {
        status.textContent = `Opening ${contextLabel}: ${segments.slice(0, i + 1).join('/')}...`;
      } else {
        // Optional status display.
      }
      const resolved = await findSegmentAcrossPages(
        children,
        seg,
        isLast,
        trailingSlash,
        remainingJoined,
        status,
        `${contextLabel}/${segments.slice(0, i).join('/')}`
      );
      if (!resolved) {
        throw new Error(`Could not find "${seg}" under ${contextLabel}${i > 0 ? '/' + segments.slice(0, i).join('/') : ''}`);
      } else {
        currentEntry = resolved.entry;
        if (resolved.consumedAll || isLast) {
          const driverOverride = selectedDriver && selectedDriver !== 'auto_driver' ? selectedDriver : null;
          if (currentEntry.handle && isExpandable(currentEntry.resource.kind) && currentEntry.resource.kind !== 'url') {
            await ensureNodeExpanded(currentEntry, driverOverride);
          } else {
            // Leaf node stops highlighted without driver expansion.
          }
          highlightTargetRow(currentEntry);
          return currentEntry;
        } else {
          if (!currentEntry.handle || !isExpandable(currentEntry.resource.kind)) {
            throw new Error(`"${currentEntry.name}" (${currentEntry.resource.kind}) cannot be expanded further to reach "${segments.slice(i + 1).join('/')}"`);
          } else {
            children = await ensureNodeExpanded(currentEntry);
          }
        }
      }
    }
    return currentEntry;
  }
}

async function openS3InTree(parsed, rootEntries, status, selectedDriver) {
  const awsEntry = rootEntries.find(e => e.name === 'aws' && e.resource.kind === 'aws');
  if (!awsEntry) {
    throw new Error('AWS browsing entry ("aws") is not available (verify that the aws CLI is installed and on PATH)');
  } else {
    status.textContent = 'Loading AWS profiles...';
    const profiles = await ensureNodeExpanded(awsEntry);
    const profileCandidates = profiles.filter(e => e.resource.kind === 'aws' && e.handle);
    if (profileCandidates.length === 0) {
      const errEntry = profiles.find(e => e.resource.kind === 'error');
      throw new Error(errEntry ? errEntry.name : 'No AWS CLI profiles found');
    } else {
      let orderedProfiles = [];
      if (parsed.profile) {
        const chosen = profileCandidates.find(p => p.name === parsed.profile);
        if (!chosen) {
          throw new Error(`AWS profile "${parsed.profile}" was not found (available: ${profileCandidates.map(p => p.name).join(', ')})`);
        } else {
          orderedProfiles = [chosen];
        }
      } else {
        const def = profileCandidates.find(p => p.name === 'default');
        const rest = profileCandidates.filter(p => p.name !== 'default');
        orderedProfiles = def ? [def].concat(rest) : rest;
      }

      let lastErr = null;
      for (const profileEntry of orderedProfiles) {
        const wasOpen = profileEntry.item.nextSibling?.className === 'tree';
        try {
          status.textContent = `Opening AWS profile ${profileEntry.name}...`;
          const profileChildren = await ensureNodeExpanded(profileEntry);
          const regionKV = profileChildren.find(c => c.resource.kind === 'kv' && c.name.startsWith('region: '));
          const configuredRegion = regionKV ? regionKV.name.slice('region: '.length).trim() : '';
          const regionsRoot = profileChildren.find(c => c.name === '[regions and resources]' && c.resource.kind === 'aws');
          if (!regionsRoot) {
            const errChild = profileChildren.find(c => c.resource.kind === 'error');
            throw new Error(errChild ? errChild.name : `No [regions and resources] under profile ${profileEntry.name}`);
          } else {
            status.textContent = `Listing AWS regions for ${profileEntry.name}...`;
            const regions = await ensureNodeExpanded(regionsRoot);
            const regionCandidates = regions.filter(r => r.resource.kind === 'aws' && r.handle);
            const targetRegion = parsed.region || configuredRegion || 'us-east-1';
            const regionEntry = regionCandidates.find(r => r.name === targetRegion || r.name.startsWith(`${targetRegion} `))
              || regionCandidates.find(r => r.name.startsWith('us-east-1 '))
              || regionCandidates[0];
            if (!regionEntry) {
              const errRegion = regions.find(r => r.resource.kind === 'error');
              throw new Error(errRegion ? errRegion.name : `No AWS regions available under profile ${profileEntry.name}`);
            } else {
              const regionChildren = await ensureNodeExpanded(regionEntry);
              const catalogEntry = regionChildren.find(c => c.name === '[service catalog]' && c.resource.kind === 'aws');
              if (!catalogEntry) {
                throw new Error(`No [service catalog] found in region ${regionEntry.name}`);
              } else {
                const services = await ensureNodeExpanded(catalogEntry);
                const s3Entry = services.find(s => s.name === 'S3 buckets' && s.resource.kind === 'aws');
                if (!s3Entry) {
                  throw new Error(`S3 buckets view was not found in [service catalog]`);
                } else {
                  status.textContent = `Looking up S3 bucket ${parsed.bucket}...`;
                  const buckets = await ensureNodeExpanded(s3Entry);
                  const bucketEntry = await findChildAcrossPages(
                    buckets,
                    b => b.name === parsed.bucket && b.resource.kind === 'aws',
                    status,
                    `S3 buckets (${profileEntry.name})`
                  );
                  if (!bucketEntry) {
                    throw new Error(`S3 bucket "${parsed.bucket}" was not found under profile "${profileEntry.name}"`);
                  } else if (parsed.segments.length === 0) {
                    const driverOverride = selectedDriver && selectedDriver !== 'auto_driver' ? selectedDriver : null;
                    await ensureNodeExpanded(bucketEntry, driverOverride);
                    highlightTargetRow(bucketEntry);
                    status.textContent = `Opened ${parsed.raw}`;
                    return;
                  } else {
                    const bucketChildren = await ensureNodeExpanded(bucketEntry);
                    await walkTreeSegments(bucketChildren, parsed.segments, parsed.trailingSlash, status, `s3://${parsed.bucket}`, selectedDriver);
                    status.textContent = `Opened ${parsed.raw}`;
                    return;
                  }
                }
              }
            }
          }
        } catch (err) {
          lastErr = err;
          if (!wasOpen && profileEntry.item.nextSibling?.className === 'tree') {
            collapseBranch(profileEntry.item);
          } else {
            // Preserve profile if it had already been opened before navigation.
          }
        }
      }
      throw lastErr || new Error(`Could not locate ${parsed.raw}`);
    }
  }
}

async function openFileInTree(parsed, rootEntries, status, selectedDriver) {
  const rootInfo = window.datatreeRootInfo || { rootDirectory: '', homeDirectory: '' };
  const targetPath = normalizeLocalPath(parsed.path);
  let startChildren = rootEntries;
  let relSegments = [];

  if (rootInfo.rootDirectory) {
    const rootDir = normalizeLocalPath(rootInfo.rootDirectory);
    if (targetPath === rootDir) {
      status.textContent = `Already viewing root directory ${rootDir}`;
      return;
    } else if (rootDir === '/' && targetPath.startsWith('/')) {
      relSegments = targetPath.slice(1).split('/').filter(Boolean);
    } else if (targetPath.startsWith(`${rootDir}/`)) {
      relSegments = targetPath.slice(rootDir.length + 1).split('/').filter(Boolean);
    } else {
      throw new Error(`Path "${targetPath}" is outside opened root directory "${rootDir}". Start Datatree without a directory argument to browse all locations.`);
    }
  } else {
    const homeDir = normalizeLocalPath(rootInfo.homeDirectory || '');
    let startEntry = null;
    if (homeDir && (targetPath === homeDir || targetPath.startsWith(`${homeDir}/`))) {
      startEntry = rootEntries.find(e => e.name === 'HOME' && e.resource.kind === 'directory');
      if (startEntry) {
        relSegments = targetPath === homeDir ? [] : targetPath.slice(homeDir.length + 1).split('/').filter(Boolean);
      } else {
        // Fall back to filesystem root if HOME entry is unavailable.
      }
    } else {
      // Check mounted volumes or filesystem root below.
    }
    if (!startEntry) {
      startEntry = rootEntries.find(e => e.name === '/' && e.resource.kind === 'directory');
      if (!startEntry) {
        throw new Error(`Filesystem root "/" is not available to open "${targetPath}"`);
      } else {
        relSegments = targetPath.slice(1).split('/').filter(Boolean);
      }
    } else {
      // Matched HOME location entry.
    }
    if (relSegments.length === 0) {
      await ensureNodeExpanded(startEntry);
      highlightTargetRow(startEntry);
      status.textContent = `Opened ${targetPath}`;
      return;
    } else {
      status.textContent = `Opening ${startEntry.name}...`;
      startChildren = await ensureNodeExpanded(startEntry);
    }
  }

  await walkTreeSegments(startChildren, relSegments, parsed.trailingSlash, status, targetPath, selectedDriver);
  status.textContent = `Opened ${parsed.raw}`;
}

async function openK8sInTree(parsed, rootEntries, status, selectedDriver) {
  const k8sEntry = rootEntries.find(e => e.name === 'kubernetes' && e.resource.kind === 'kubernetes');
  if (!k8sEntry) {
    throw new Error('Kubernetes browsing entry ("kubernetes") is not available');
  } else {
    status.textContent = 'Loading Kubernetes contexts...';
    const contexts = await ensureNodeExpanded(k8sEntry);
    await walkTreeSegments(contexts, parsed.segments, parsed.trailingSlash, status, 'k8s:/', selectedDriver);
    status.textContent = `Opened ${parsed.raw}`;
  }
}

window.openTreeURL = async function openTreeURL(rawURL, statusElement, selectedDriver = '') {
  const status = statusElement || document.getElementById('url-status');
  const parsed = parseNavigableURL(rawURL);
  if (!parsed) {
    throw new Error(`"${rawURL}" is not a tree-navigable URL`);
  } else if (parsed.scheme === 'invalid') {
    status.className = 'error';
    status.textContent = parsed.error;
    throw new Error(parsed.error);
  } else {
    const clearBtn = document.getElementById('speedsearch-clear');
    if (clearBtn && !clearBtn.hidden) {
      clearBtn.click();
    } else {
      // No active Speedsearch filter to clear.
    }
    const rootEntries = window.rootRenderedResources || [];
    if (rootEntries.length === 0) {
      const msg = 'Tree is not loaded yet; wait a moment and try again';
      status.className = 'error';
      status.textContent = msg;
      throw new Error(msg);
    } else {
      status.className = 'meta';
      try {
        if (parsed.scheme === 's3') {
          await openS3InTree(parsed, rootEntries, status, selectedDriver);
        } else if (parsed.scheme === 'file') {
          await openFileInTree(parsed, rootEntries, status, selectedDriver);
        } else if (parsed.scheme === 'k8s') {
          await openK8sInTree(parsed, rootEntries, status, selectedDriver);
        } else {
          throw new Error(`Unsupported tree URL scheme "${parsed.scheme}"`);
        }
      } catch (error) {
        status.className = 'error';
        status.textContent = `Could not open ${rawURL}: ${error.message}`;
        throw error;
      }
    }
  }
};
