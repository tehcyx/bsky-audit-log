# bsky audit trail backup

This repository backs up my
[follower list](followers.txt),
[following list](following.txt),
[blocked accounts list](blocked_accounts.txt) and
[muted accounts list](mutes.txt) periodically using GitHub Actions.

## Set up

1. Fork this repository.
1. `git rm *.txt` and commit.
1. Create a bsky app password (https://staging.bsky.app/settings/app-passwords).
1. Go to Repository Settings &rarr; Secrets and add secrets to be able to connect to bsky:

   - BSKY_HANDLE
   - BSKY_APP_PWD


1. Go to Repository Settings &rarr; Secrets and add the base url of your bsky instance:

   - BSKY_INSTANCE

1. See [.github/workflows/update.yaml](/.github/workflows/update.yaml) and modify the cron schedule (in UTC) as you see fit.

1. Commit and push. Once the time arrives, the cron would work, and commit the lists into `.txt` files and push the changes.

1. Update the [Github Action permissions](https://github.com/tehcyx/bsky-audit-log/settings/actions) to allow write access to the repo:
![image](https://github.com/tehcyx/bsky-audit-log/assets/917785/ee50e3ef-3bdf-4245-acb6-678a388ec682)

