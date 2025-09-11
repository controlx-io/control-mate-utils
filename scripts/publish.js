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

  // Build the application
  console.log('🔨 Building application...');
  execSync('npm run build', { stdio: 'inherit' });

  // Verify build output exists
  const binaryPath = './release/control-mate-utils';
  if (!fs.existsSync(binaryPath)) {
    console.error('❌ Build failed: Binary not found at', binaryPath);
    process.exit(1);
  }

  // Create release notes
  const releaseNotes = `## Control Mate Utils ${currentVersion}

Network and resource management utility for ControlMate PC.

### Installation
1. Download the \`control-mate-utils\` binary
2. Make it executable: \`chmod +x control-mate-utils\`
3. Run: \`./control-mate-utils\`

The application will start on port 8080.`;

  // Create GitHub release
  console.log('🚀 Creating GitHub release...');
  const releaseCommand = [
    'gh release create',
    tagName,
    binaryPath,
    '--title', `Control Mate Utils ${currentVersion}`,
    '--notes', `"${releaseNotes}"`,
    '--latest'
  ].join(' ');

  execSync(releaseCommand, { stdio: 'inherit' });

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
    execSync(`git push origin ${tagName}`, { stdio: 'inherit' });
    console.log(`✅ Pushed tag ${tagName} to remote`);
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
