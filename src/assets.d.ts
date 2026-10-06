// Bun imports these as strings (`with { type: 'text' }`), also inside `bun build --compile`.
declare module '*.md' {
  const content: string;
  export default content;
}
