# os_updates

**OS updates** come from the Hetzner Debian image itself: `unattended-upgrades` is enabled
and installs security and stable updates every day, from Debian and Debian-Security. This
role only moves *when*: it moves `apt-daily-upgrade.timer` (Debian's default is 06:00 on the
node's clock plus up to an hour) with a drop-in,
`/etc/systemd/system/apt-daily-upgrade.timer.d/50-ansible.conf`, to 03:30 Zurich time (plus
up to 15 minutes), after the k3s upgrade window and before kured's reboot window - see "The
night's maintenance order" in [`AGENTS.md`](../../../AGENTS.md). systemd evaluates the time
zone in `OnCalendar` itself.

- **The package lists are fetched at 03:00** (plus up to 5 minutes), once a day:
  `apt-daily.timer`, with a drop-in of the same shape. unattended-upgrades installs only what
  the last fetch found, and Debian fetches twice a day at random times (06:00 and 18:00 plus
  up to 12 hours). So each node installed from lists of a different age: kernel 6.12.113
  reached prod-03 a night before the others (2026-10-17). Fetched within the same few
  minutes, the lists bring every node to the same state each night, and kured reboots all
  that need it in the same window. The fetch also downloads the packages, so the 03:30 run
  only installs; it changes nothing on the node, so it may overlap the k3s upgrades.
- Debian's `Automatic-Reboot` stays off: it would reboot every node at the same time.
  Reboots are kured's.

## Running it

Tagged `os_updates`:

```shell
cd ansible
ansible-playbook prod.yml --tags os_updates
ansible prod -m command -a 'systemctl list-timers apt-daily.timer apt-daily-upgrade.timer --no-pager'   # next runs, in UTC
```
