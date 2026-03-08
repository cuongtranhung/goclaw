import { useState } from "react";
import {
  Folder,
  FolderOpen,
  FileText,
  FileCode2,
  File,
  ChevronRight,
  Loader2,
} from "lucide-react";
import { extOf, CODE_EXTENSIONS, type TreeNode } from "./skill-file-helpers";

function FileIcon({ name }: { name: string }) {
  const ext = extOf(name);
  if (ext === "md") return <FileText className="h-4 w-4 shrink-0 text-blue-500" />;
  if (CODE_EXTENSIONS.has(ext)) return <FileCode2 className="h-4 w-4 shrink-0 text-orange-500" />;
  return <File className="h-4 w-4 shrink-0 text-muted-foreground" />;
}

export function TreeItem({
  node,
  depth,
  activePath,
  onSelect,
}: {
  node: TreeNode;
  depth: number;
  activePath: string | null;
  onSelect: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(depth === 0);
  const isActive = activePath === node.path;

  if (node.isDir) {
    return (
      <div>
        <button
          type="button"
          className="flex w-full items-center gap-1 rounded px-2 py-1 text-left text-sm hover:bg-accent cursor-pointer"
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
          onClick={() => setExpanded(!expanded)}
        >
          <ChevronRight
            className={`h-3 w-3 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`}
          />
          {expanded ? (
            <FolderOpen className="h-4 w-4 shrink-0 text-yellow-600" />
          ) : (
            <Folder className="h-4 w-4 shrink-0 text-yellow-600" />
          )}
          <span className="truncate">{node.name}</span>
        </button>
        {expanded && node.children.map((child) => (
          <TreeItem
            key={child.path}
            node={child}
            depth={depth + 1}
            activePath={activePath}
            onSelect={onSelect}
          />
        ))}
      </div>
    );
  }

  return (
    <button
      type="button"
      className={`flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-sm cursor-pointer ${
        isActive ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
      }`}
      style={{ paddingLeft: `${depth * 16 + 20}px` }}
      onClick={() => onSelect(node.path)}
    >
      <FileIcon name={node.name} />
      <span className="truncate">{node.name}</span>
    </button>
  );
}

export function FileTreePanel({
  tree,
  filesLoading,
  activePath,
  onSelect,
}: {
  tree: TreeNode[];
  filesLoading: boolean;
  activePath: string | null;
  onSelect: (path: string) => void;
}) {
  if (filesLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (tree.length === 0) {
    return <p className="px-3 py-4 text-sm text-muted-foreground">No files found.</p>;
  }
  return (
    <>
      {tree.map((node) => (
        <TreeItem key={node.path} node={node} depth={0} activePath={activePath} onSelect={onSelect} />
      ))}
    </>
  );
}
