// Expanded trees remember their own resource row; sibling and ancestor trees
// remain independent when one branch is removed.
function rememberBranch(tree, item, name) {
  tree.branchOwner = item;
  tree.branchName = name;
  tree.branchDepth = (item.closest('.tree')?.branchDepth || 0) + 1;
}

function collapseBranch(item) {
  const tree = item.nextElementSibling;
  if (tree?.classList.contains('tree') && tree.branchOwner === item) {
    if (tree.foldByHiding) {
      tree.hidden = true;
    } else {
      for (const node of [tree, ...tree.querySelectorAll('.tree')]) {
        if (node.previewURL) {
          URL.revokeObjectURL(node.previewURL);
        } else {
          // Text-only branches allocate no media URLs.
        }
      }
      tree.remove();
    }
    setExpansionState(item, false);
    return true;
  } else {
    return false;
  }
}

function visibleBranch() {
  const readingLine = Math.min(window.innerHeight * 0.3, 160);
  let selected = null;
  let nearest = Infinity;
  for (const tree of document.querySelectorAll('.tree')) {
    if (tree.branchOwner?.isConnected) {
      const bounds = tree.getBoundingClientRect();
      if (bounds.bottom > 0 && bounds.top < window.innerHeight && bounds.height > 0) {
        const distance = Math.max(bounds.top - readingLine, readingLine - bounds.bottom, 0);
        if (distance < nearest || (distance === nearest && tree.branchDepth > (selected?.branchDepth || 0))) {
          selected = tree;
          nearest = distance;
        } else {
          // Keep the closest, deepest visible branch.
        }
      } else {
        // Entirely offscreen branches are not current reading locations.
      }
    } else {
      // Root trees and closed upload containers have no collapsible owner.
    }
  }
  return selected;
}

document.addEventListener('DOMContentLoaded', () => {
  const button = document.getElementById('fold-current');
  let current = null;
  let queued = false;
  function update() {
    queued = false;
    current = visibleBranch();
    button.hidden = current === null;
    if (current) {
      button.textContent = `↑ Fold up: ${current.branchName}`;
      button.title = `Collapse only “${current.branchName}” and return to its row. Other branches stay open.`;
      button.setAttribute('aria-label', `Fold up ${current.branchName}`);
    } else {
      button.textContent = '↑ Fold up';
      button.title = 'Scroll into an expanded branch to fold up that branch only';
    }
  }
  function queueUpdate() {
    if (queued) {
      return;
    } else {
      queued = true;
      requestAnimationFrame(update);
    }
  }
  button.addEventListener('click', () => {
    // Recheck at click time so a recent scroll cannot fold the old target.
    const branch = visibleBranch();
    if (branch && collapseBranch(branch.branchOwner)) {
      branch.branchOwner.scrollIntoView({block: 'center', behavior: 'instant'});
      branch.branchOwner.querySelector('.expander button')?.focus({preventScroll: true});
      queueUpdate();
    } else {
      update();
    }
  });
  const observer = new MutationObserver(queueUpdate);
  for (const id of ['tree', 'upload-tree']) {
    observer.observe(document.getElementById(id), {childList: true, subtree: true});
  }
  document.addEventListener('scroll', queueUpdate, {capture: true, passive: true});
  window.addEventListener('resize', queueUpdate);
  update();
});
