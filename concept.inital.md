# bjarne Interactive UI

A Claude Code-inspired terminal UI for C++ code generation and validation.

## **Concept overview**

Bjarne CLI code editor takes AI code gen tools to the next level. What this product does is run any code generated through a full test suite before responding to the user. We are going to start with C/C++ but will expand this to other languages as we have validated the concept with C/C++. I launch bjarne, I prompt it to generate C/C++ code, but before I get the answer, Bjarne app pipes the code received from the LLM and sends it to a container which has clang-tidy, compilation engine, ASAN, TSAN, Runtime engine so that the code has to run through all these gates and only once it's passed this validation process does it get back to the user. The tools, clang-tidy, asan, tsan all output verbose logging, when failure is detected, the verbose log is sent back to the llm for processing and the LLM returns the code with just the diff change not a complete rewrite. The tool starts with Haiku 4.5, if that fails, sonnet 4.5 takes over to have an attempt to generate clean C/C++ code, if after 2 rounds it can't use Opus 4.5 for the remaining iterations. Like to see some logic that evaluates the ask first, if the ask is medium complexity, just go straight to sonnet and if high complexity just go straight to opus 4.5. After 10 tries asked the user if they want to keep trying. Part of the initial prompt is to evaluate the prompt and ask clarifying questions, if the user is asking for C/C++ function or code that is so complex that gcc or clang aren't able to compile, meaning building C/C++ that is functioning within the limits of the current build tool chain. So bjarne will clarify if they want to maybe scale back their prompt. Security checks as well if there is such a library from snyk for instance. Use Podman for stateless container,

## Features

### 🎨 Multiple Themes
- **Default**: Unix/dev operator color palette
- **Matrix**: Monochrome green-on-black (classic terminal)
- **Solarized Dark**: Balanced, low-contrast theme
- **Gruvbox Dark**: Warm, earthy tones
- **Dracula**: Purple-tinted dark theme
- **Nord**: Cool blue-gray minimalist palette

### 🔒 Trust System
- Security prompt on first launch
- Folder-level trust settings
- Persistent configuration in `.bjarne/settings.local.json`

### 💬 Conversational Interface
- Chat-style REPL with message history
- Syntax-highlighted C++ code snippets
- File creation approval dialogs
- Interactive command menu

### ⚡ Commands
- `/theme` - Change color theme
- `/exit` - Exit the application
- `/help` - Show available commands
- `/init` - Create bjarne.md file
- `/doctor` - Diagnose installation

## Installation

1. Install dependencies:
```bash
npm install
```

2. Compile TypeScript:
```bash
npx tsc
```

3. Run the UI:
```bash
# In an interactive terminal
npx tsx bjarne-ui.tsx

# Or with compiled JavaScript
node bjarne-ui.js
```

## Architecture

### Component Structure
```
src/ui/
├── theme.ts              # Current theme configuration
├── themes.ts             # All theme definitions
├── ThemeManager.ts       # Theme switching logic
├── ThemeSelector.tsx     # Theme selection UI
├── REPL.tsx              # Main conversational interface
├── TrustPrompt.tsx       # Security dialog
├── CommandMenu.tsx       # Slash command menu
├── FileApprovalDialog.tsx # File creation approval
├── CodeHighlighter.tsx   # c/C++ syntax highlighting
└── TrustPrompt.tsx       # Trust settings dialog
```

### Key Technologies
- **Ink**: React for CLI applications
- **TypeScript**: Full type safety
- **Yoga Layout**: Flexbox in the terminal
- **React Hooks**: Modern component patterns

## Theme Customization

To add a new theme, edit `src/ui/themes.ts`:

```typescript
export const myTheme: Theme = {
  name: 'My Theme',
  description: 'Custom color scheme',
  background: '#000000',
  foreground: '#ffffff',
  primary: '#00ff00',
  // ... other colors
};

// Add to theme registry
export const themes = {
  // ... existing themes
  'my-theme': myTheme,
};
```

## Development

### Running Tests
```bash
npm test
```

### Type Checking
```bash
npx tsc --noEmit
```

### Building for Distribution
```bash
npm run build
```

## UI Screenshots

### Welcome Screen
```
┌────────────────────────┐
│                        │
│      bjarne v0.1.0     │
│                        │
│  Hatching code since   │
│   2025                 │
│                        │
│   Working directory:   │
│   c:\Dev\Github\bjarne │
└────────────────────────┘
```

### Chat Interface
```
> hello
● Hello! I'm Bjarne, ready to help you generate
  and validate Bjarne. How can I assist you today?

> create a hello world program in c++
● I'll create a simple C++ hello world program for you.

  ● Write(hello.cpp)
└ Created hello.cpp

> /theme
┌─────────────────────────────────────────────┐
│ Select Theme                                │
│ Choose your preferred color scheme          │
│                                             │
│ ● Default     Unix/dev operator palette    │
│   Matrix      Monochrome green-on-black    │
│   Solarized   Balanced, low-contrast       │
│   Gruvbox     Warm, earthy tones          │
│   Dracula     Purple-tinted dark          │
│   Nord        Cool blue-gray minimalist   │
│                                             │
│ ↑↓ Navigate · Enter Select · Esc Cancel    │
└─────────────────────────────────────────────┘
```

## Notes

- The UI requires an interactive terminal (TTY)
- Theme changes take effect on next launch
- Settings are stored in `.bjarne/settings.local.json`
- /init creates bjarne.md, scans directory and builds out overview .md
- All file operations require user approval by default

## Future Enhancements

- [ ] Real-time theme switching without restart
- [ ] Custom key bindings
- [ ] Multi-tab support
- [ ] Integrated Docker validation pipeline
- [ ] AWS Bedrock integration for AI responses
- [ ] Syntax highlighting for more languages
- [ ] Export conversation history
- [ ] Plugin system for custom commands