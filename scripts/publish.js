#!/usr/bin/env node

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

// Read package.json to get current version
const packageJson = JSON.parse(fs.readFileSync('package.json', 'utf8'));
const currentVersion = packageJson.version;
const tagName = `v${currentVersion}`;

  console.log(`Checking if version ${currentVersion} is already released...`);

try {
  // Check for uncommitted changes
  console.log('🔍 Checking for uncommitted changes...');
  try {
    const gitStatus = execSync('git status --porcelain', { encoding: 'utf8' });
    if (gitStatus.trim()) {
      console.error('❌ There are uncommitted changes. Please commit all changes before releasing.');
      console.error('Uncommitted files:');
      console.error(gitStatus);
      process.exit(1);
    }
    console.log('✅ No uncommitted changes found.');
  } catch (error) {
    console.error('❌ Failed to check git status:', error.message);
    process.exit(1);
  }

  // Check if GitHub CLI is installed
  try {
    execSync('gh --version', { stdio: 'ignore' });
  } catch (error) {
    console.error('❌ GitHub CLI (gh) is not installed.');
    console.error('Please install it first:');
    console.error('  macOS: brew install gh');
    console.error('  Linux: https://github.com/cli/cli#installation');
    process.exit(1);
  }

  // Check if user is authenticated with GitHub
  try {
    execSync('gh auth status', { stdio: 'ignore' });
  } catch (error) {
    console.error('❌ Not authenticated with GitHub CLI.');
    console.error('Please run: gh auth login');
    process.exit(1);
  }

  // Check if the tag already exists
  try {
    execSync(`gh release view ${tagName}`, { stdio: 'ignore' });
    console.log(`✅ Version ${currentVersion} is already released.`);
    console.log('Skipping publish to avoid duplicate releases.');
    process.exit(0);
  } catch (error) {
    // Tag doesn't exist, continue with release
    console.log(`📦 Version ${currentVersion} not found. Proceeding with release...`);
  }

  // Build the application for both architectures
  console.log('🔨 Building application for both architectures...');
  execSync('npm run build', { stdio: 'inherit' });

  // Verify build outputs exist
  const amd64BinaryPath = './release/cm-utils-linux-amd64';
  const arm64BinaryPath = './release/cm-utils-linux-arm64';
  
  if (!fs.existsSync(amd64BinaryPath)) {
    console.error('❌ Build failed: AMD64 binary not found at', amd64BinaryPath);
    process.exit(1);
  }
  
  if (!fs.existsSync(arm64BinaryPath)) {
    console.error('❌ Build failed: ARM64 binary not found at', arm64BinaryPath);
    process.exit(1);
  }

  // Create release notes
  const releaseNotes = `## ControlMate Utils ${currentVersion}

Network and resource management utility for ControlMate PC.

### Installation
1. Download the appropriate binary for your architecture:
   - \`cm-utils-linux-amd64\` for x86_64 systems
   - \`cm-utils-linux-arm64\` for ARM64 systems
2. Make it executable: chmod +x \`cm-utils-linux-*\`
3. Run: \`sudo ./cm-utils-linux-*\`

### Linux Installation (Recommended)
\`\`\`bash
curl -sL https://raw.githubusercontent.com/controlx-io/control-mate-utils/refs/heads/main/scripts/install_to_linux.sh | sudo -E bash -
\`\`\`

The application will start on port 8080.`;

  // Create GitHub release
  console.log('🚀 Creating GitHub release...');
  
  // Write release notes to a temporary file to avoid shell escaping issues
  const notesFile = './release-notes.tmp';
  fs.writeFileSync(notesFile, releaseNotes);
  
  try {
    execSync(`gh release create ${tagName} ${amd64BinaryPath} ${arm64BinaryPath} --title "ControlMate Utils ${currentVersion}" --notes-file ${notesFile} --latest`, { 
      stdio: 'inherit' 
    });
  } finally {
    // Clean up temporary file
    if (fs.existsSync(notesFile)) {
      fs.unlinkSync(notesFile);
    }
  }

  // Tag the current commit
  console.log('🏷️  Creating git tag...');
  try {
    execSync(`git tag ${tagName}`, { stdio: 'inherit' });
    console.log(`✅ Created tag ${tagName}`);
  } catch (error) {
    console.error('❌ Failed to create git tag:', error.message);
    process.exit(1);
  }

  // Push the tag to remote
  console.log('📤 Pushing tag to remote...');
  try {
    // Get the remote name (usually 'origin' or 'main')
    const remoteName = execSync('git remote', { encoding: 'utf8' }).trim().split('\n')[0];
    execSync(`git push ${remoteName} ${tagName}`, { stdio: 'inherit' });
    console.log(`✅ Pushed tag ${tagName} to remote (${remoteName})`);
  } catch (error) {
    console.error('❌ Failed to push tag:', error.message);
    process.exit(1);
  }

  console.log(`✅ Successfully released version ${currentVersion}!`);
  console.log(`🔗 View release: https://github.com/controlx-io/control-mate-utils/releases/tag/${tagName}`);

} catch (error) {
  console.error('❌ Publish failed:', error.message);
  process.exit(1);
}
