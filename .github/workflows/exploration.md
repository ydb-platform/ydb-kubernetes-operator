I could trigger action when PR has been approved like this:
https://github.com/orgs/community/discussions/25372

Problem is, it does NOT import secrets.

I have another plan

We use labels.
When I come as a member, my pipelines are run and nobody gets disturbed.

When I come as a non-member,
- no pipelines are run
- comment about labels is posted
- labels are disallowed to be put by external contributors
- when maintainer puts a label, github pipeline runs and everyone is happy
- maintainer doesn't have to manually register, semantic meaning of approval is not flawed, and 
  I pray to god it can be achieved without using a PAT.

  notify-contributor:
    name: post-comment
    runs-on: ubuntu-latest
    outputs:
      if: >- 
        ${{ !(github.event.action == "opened" && 
              github.event.pull_request.author_association == "MEMBER") &&
            !(github.event.action == "labeled" && 
              github.event.label.name == "ok-to-test") }}
      steps:
        
