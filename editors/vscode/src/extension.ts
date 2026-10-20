import { workspace, ExtensionContext } from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from 'vscode-languageclient/node';

let client: LanguageClient;

export function activate(_context: ExtensionContext) {
  const config = workspace.getConfiguration('goozels');
  const command = config.get<string>('path') || 'goozels';
  const goplsPath = config.get<string>('gopls') || '';

  const args: string[] = [];
  if (goplsPath) {
    args.push('--gopls', goplsPath);
  }

  const serverOptions: ServerOptions = {
    command,
    args,
    transport: TransportKind.stdio,
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: 'file', language: 'go' },
      { scheme: 'file', pattern: '**/gooze.yaml' },
    ],
  };

  client = new LanguageClient('goozels', 'Goozels', serverOptions, clientOptions);
  client.start();
}

export function deactivate(): Thenable<void> | undefined {
  return client?.stop();
}
