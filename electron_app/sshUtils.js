const fs = require('fs');
const path = require('path');
const SSHConfig = require('ssh-config');

function getSSHHosts() {
  try {
    const configPath = path.join(process.env.HOME || process.env.USERPROFILE, '.ssh', 'config');
    const data = fs.readFileSync(configPath, 'utf8');
    const parsed = SSHConfig.parse(data);
    return parsed
      .filter((item) => item.type === SSHConfig.DIRECTIVE && item.param === 'Host')
      .map((item) => item.value)
      .filter((host) => host !== '*');
  } catch (err) {
    return [];
  }
}

module.exports = { getSSHHosts };
