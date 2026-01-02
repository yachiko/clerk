Plan creation of simple Goland CLI tool that will help managing secrets in AWS Parameter Store. The tool will allow users to perform basic operations such as adding, retrieving, updating, and deleting secrets stored in AWS Parameter Store.

# Tech stack 
- Language: Go (latest stable).  
- CLI: `spf13/cobra`.  
- UI: `manifoldco/promptui`.  
- Styling: `fatih/color`.  
- AWS: `aws-sdk-go-v2` 
- Progress bar: `schollz/progressbar` 

# Features

- add/update secret: `clerk put "/dev/db_password" secret.json` or `clerk put "/dev/db_password" "mypasssword123"` , this will create a new secret or update an existing one. In output we will show version of the secret. Tags can be added via --tags flag in format key1=value1,key2=value2. Type of the secret (String, StringList, SecureString) can be specified via --type flag, default is SecureString, configurable via config file or env variable. KMS key ID can be specified via --kms-key-id flag, default is AWS managed key for Parameter Store. Secret.json contains raw secret value.
- delete secret: `clerk delete "/dev/db_password"` , this will delete the specified secret from AWS Parameter Store. In output we will show confirmation message, ask for confirmation before deleting (--force to skip confirmation).
- browse secrets: `clerk browse`, this starts an interactive terminal UI to navigate through secrets stored in AWS Parameter Store. Users can select a secret to view its details like value, version history, and metadata. This feature will be further specified in dedicated paragraph.
- get secret: `clerk get "/dev/db_password"` , this will retrieve the value of the specified secret. In output we will show the secret value in a secure manner (e.g., masked or encrypted). Decrypted by default, --mask flag to show masked value. Use '@' for specifing version (e.g., /dev/db_password@3 to get version 3), support '@latest` to get latest version, but it's default behavior. By default return whole secret structure (name, value, version, created date, modified date, tags), use `--value` flag to return only secret value.
- move and copy secret: `clerk mv "/dev/old_secret" "/prod/new_secret"` and `clerk cp "/dev/secret" "/prod/secret_copy"` to move or copy secrets within Parameter Store. In output we will show confirmation message along with new secret version.
- list secrets: `clerk list "/dev/*"` to list all secrets under specified path. Support glob patterns (e.g., /dev/* to match all secrets under /dev/). In output we will show list of secret names along with their versions and tags. Support `--sort` flag to sort by name, created date, modified date, have short aliases for those (n, c, m).
- refresh cache: `clerk refresh` to manually refresh the local cache of secret names and metadata from AWS Parameter Store. Show progress bar during refresh operation.
- config: `clerk config set|get <key> [value]` to set or get configuration options like default region, profile, cache location, cache TTL, clipboard clear timeout.
- help: `clerk --help` to display help information about the CLI tool and its commands.
- completion: support shell completion for bash, zsh, fish.
- version: `clerk --version` to display the current version of the CLI tool.

# The browse feature

It's the main feature of the tool, allowing users to interactively explore and manage their secrets stored in AWS Parameter Store. The browse feature will provide a user-friendly terminal UI that enables users to navigate through their secrets, view details, and perform actions such as updating or deleting secrets. It should enable quick search and filtering based on name or tags. Caching will be needed to improve performance when dealing with a large number of secrets. (clerk refresh to update cache). Follow similar principle like k9s tool for kubernetes, when user can navigate using keyboard arrows, then click a key to perform an action (d for describe, e for edit, del for delete, q for quit, / for search). Edit should open the secret in vscode or default $EDITOR.

Browse view should just show secret names by default, with option to toggle tree view (by 't' key) to show secrets in hierarchical structure based on their paths. In tree view, user should be able to collapse/expand nodes using space key.

In describe view we should show secret metadata like version history, created date, modified date, tags. Users can navigate through versions using tab/shift+tab keys. The value panel supports vertical scrolling with arrow keys and page up/down. For long lines, users can either enable line wrapping with the 'w' key or scroll horizontally using left/right arrow keys or shift+scroll wheel. Fetch version history metadata when entering describe view.

Editing and saving a secret should create a new version in AWS Parameter Store.

Describe view should show show secret encypted by default, with option to show masked value (--mask). When user clicks 'c' in describe view, the secret value should be copied to clipboard. 

Hitting 'esc' in describe view should return user to main browse view, string in search box should be preserved.

Page up/page down and arrow keys (left/right) should be supported in browse view to navigate through long lists of secrets

Search should support glob patterns (e.g., /dev/* to match all secrets under /dev/)

Support 'c' key in browse view to copy latest secret value to clipboard without opening describe view.

# The cache 

The tool needs to implement robust caching mechanism to store secret names locally to improve performance when browsing and searching through secrets. The cache will be stored in a local file (e.g., JSON or YAML format) and will be updated whenever the user performs operations that modify the secrets (add, update, delete). The cache should also have a refresh command to manually update the cache from AWS Parameter Store. The cache file location can be configurable via environment variable or config file. Cache TTL (time-to-live) should be implemented to automatically refresh the cache after a certain period. 

Have lock on the cache file to prevent concurrent writes.

The refresh process also should be optimized for parallel fetching of secrets metadata to speed up the process. The values of the secrets should not be cached, only names and metadata (tags, version, created date, modified date).

# AWS Configuration & Authentication

Use standard AWS SDK v2 configuration methods to authenticate and configure access to AWS Parameter Store. Support specification of AWS profile via --profile flag and region via --region flag. If not specified, default AWS SDK behavior should be followed (environment variables, shared config file, etc.). Print meaningful error messages if authentication or configuration fails.

# Error Handling & Edge Cases

Fail quickly with meaningful error messages. Prefer failing on AWS SDK calls rather than pre-validating inputs. Handle network errors, AWS service errors, and invalid inputs gracefully. Ensure that the tool does not crash unexpectedly and provides users with clear guidance on how to resolve issues.

# Parameter Store Specifics 

Handle different parameter types (String, StringList, SecureString). For SecureString, ensure that the tool handles encryption and decryption properly using AWS KMS.

# Input Validation

No not validate secret names, if API call fails due to invalid name, print the error from AWS SDK.

# Browse Feature Gaps

- Pagination: Implement pagination in the browse feature to handle large number of secrets efficiently.
- Flat list view by default, with option to switch to tree view. Toggle with 't' key. Collapse/expand using 'space' key. 
- Sorting: Allow users to sort secrets based on different criteria (name, date created, date modified).
- No bulk operations support.

# Security Concerns
- Ensure that secret values are handled securely in memory and not logged or exposed unnecessarily.
- Temp file is a potential security risk, ensure that if temp files are used (e.g., for editing secrets), they are securely deleted after use. Make best effort to use in-memory editing if possible.
- Clear clipboard after a certain period after copying secret value to clipboard (default 60 seconds).

# Config 

Support `clerk config` command to set and get configuration options like default region, profile, cache location, cache TTL, clipboard clear timeout. Config should be stored in a local config file (e.g., YAML or JSON format) in user's home directory.

# Documentation & Help 

Generate comprehensive documentation for the CLI tool, including installation instructions, usage examples for each command, and explanations of configuration options. Implement `--help` flag for each command to provide users with quick access to usage information directly from the CLI.

# Keyboard Shortcuts

**Browse View:**
- 'c' - copy secret value to clipboard
- 'd' - describe secret
- 'e' - edit secret
- 'del' - delete secret
- 'q' - quit
- '/' - search
- 't' - toggle tree/flat view
- 'space' - collapse/expand tree node (tree view)

**Describe View:**
- 'tab' / 'shift+tab' - navigate to next/previous version
- '↑' / '↓' - scroll value vertically
- '←' / '→' - scroll value horizontally (when line wrap is off)
- 'pgup' / 'pgdown' - page up/down in value
- 'w' - toggle line wrapping
- 'x' - toggle mask/unmask secret value
- 'c' - copy secret value to clipboard
- 'esc' - return to browse view
- 'q' - quit

**Mouse Support (Describe View):**
- Scroll wheel - scroll value vertically
- Shift + scroll wheel - scroll value horizontally (when line wrap is off)

# Default Config Values

- default region: us-east-1
- default profile: default
- cache location: $HOME/.clerk/cache.json
- cache TTL: 3h
- clipboard clear timeout: 60s
- default parameter type: SecureString
- default sort order for list command: by name
- parallel fetches for refresh command: 10

Config kep in json file located at $HOME/.clerk/config.json. 

# Exit codes 

- 0: Success
- 1: General error
- 2: AWS SDK error
- 3: Invalid input

# Misc

Only Parameter Store is supported, no support for AWS Secrets Manager.
Single region only.
Handle Ctrl+c gracefully, exit the tool without leaving temp files or corrupted state.
Only plain and json output formats are supported.
The project artifact should be a single binary executable for easy distribution and installation.
Package Github repo will be named github.com/yachiko/clerk.