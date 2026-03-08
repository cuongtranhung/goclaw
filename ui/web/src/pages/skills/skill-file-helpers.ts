import type { SkillFile } from "@/types/skill";

export interface TreeNode {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  children: TreeNode[];
}

export const CODE_EXTENSIONS = new Set([
  "ts", "tsx", "js", "jsx", "py", "go", "json", "yaml", "yml", "toml",
  "sh", "bash", "css", "html", "sql", "rs", "rb", "java", "kt", "swift",
  "c", "cpp", "h", "xml", "graphql", "proto", "lua", "zig", "env",
]);

export function extOf(name: string): string {
  const dot = name.lastIndexOf(".");
  return dot >= 0 ? name.slice(dot + 1).toLowerCase() : "";
}

export function langFor(ext: string): string {
  const map: Record<string, string> = {
    ts: "typescript", tsx: "tsx", js: "javascript", jsx: "jsx",
    py: "python", go: "go", rs: "rust", rb: "ruby", sh: "bash", bash: "bash",
    yml: "yaml", yaml: "yaml", json: "json", toml: "toml",
    css: "css", html: "html", sql: "sql", xml: "xml",
    java: "java", kt: "kotlin", swift: "swift",
    c: "c", cpp: "cpp", h: "c", graphql: "graphql",
    proto: "protobuf", lua: "lua", zig: "zig",
  };
  return map[ext] ?? ext;
}

export function buildTree(files: SkillFile[]): TreeNode[] {
  const root: TreeNode[] = [];
  const dirMap = new Map<string, TreeNode>();

  const sorted = [...files].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    return a.path.localeCompare(b.path);
  });

  for (const f of sorted) {
    const node: TreeNode = {
      name: f.name,
      path: f.path,
      isDir: f.isDir,
      size: f.size,
      children: [],
    };

    if (f.isDir) {
      dirMap.set(f.path, node);
    }

    const parentPath = f.path.includes("/")
      ? f.path.slice(0, f.path.lastIndexOf("/"))
      : "";

    if (parentPath && dirMap.has(parentPath)) {
      dirMap.get(parentPath)!.children.push(node);
    } else {
      root.push(node);
    }
  }

  const sortChildren = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    for (const n of nodes) {
      if (n.children.length > 0) sortChildren(n.children);
    }
  };
  sortChildren(root);
  return root;
}

export function stripFrontmatter(content: string): string {
  if (!content.startsWith("---")) return content;
  const end = content.indexOf("---", 3);
  if (end < 0) return content;
  return content.slice(end + 3).replace(/^\n+/, "");
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
