# os_updates

**OS updates** come from the Hetzner Debian image itself: `unattended-upgrades` is enabled
and installs security and stable updates every day, from Debian and Debian-Security. This
role only moves *when*: it moves `apt-daily-upgrade.timer` (Debian's default is 06:00 on the
node's clock plus up to an hour) with a drop-in,
`/etc/systemd/system/apt-daily-upgrade.timer.d/50-ansible.conf`, to 03:30 Zurich time (plus
up to 15 minutes), after the k3s upgrade window and before kured's reboot window - see "The
night's maintenance order" in [`AGENTS.md`](../../../AGENTS.md). systemd evaluates the time
zone in `OnCalendar` itself.

- `apt-daily.timer`, which refreshes the package lists twice a day, is left alone.
- Debian's `Automatic-Reboot` stays off: it would reboot every node at the same time.
  Reboots are kured's.

## Running it

Tagged `os_updates`:

```shell
cd ansible
ansible-playbook prod.yml --tags os_updates
ssh prod-01 systemctl list-timers apt-daily-upgrade.timer   # next run, in UTC
```
