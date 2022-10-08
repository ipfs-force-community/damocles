<p align="center">
  <a href="https://venus.filecoin.io/" title="Filecoin Docs">
    <img src="docs/images/venus_logo_big2.jpg" alt="Project Venus Logo" width="330" />
  </a>
</p>


<h1 align="center">venus-cluster</h1>

`venus-cluster` is a solution for `Filecoin` storage power clusters, focusing on growing and maintaining said storage power. Compared with `lotus`, it tries to make breakthroughs in the following aspects:

- Simplifies sealing state machine
- More efficient usage of resources through optimized procedures
- Taking advantages of Venus chain service and Venus storage deal service to maximize storage providing performance
- Multi miner_id support and collaboration  

`venus-cluster` is composed of `venus-worker` and `venus-sector-manager`. The former is used for `PoRep` computations and latter is used for sector management and interactions with Filecoin chain.

## Building & Documentation

For instructions on how to quickly bootstrap your storage power service using `venus-cluster`, please visit [here](https://venus.filecoin.io/cluster/getting-started.html).

For instructions on more intricate details of `venus-cluster` configurations, please visit [here](https://github.com/ipfs-force-community/venus-cluster/tree/main/docs).

## Venus Architecture

Venus architecture mianly compose of three parts: chain service, storage deal service and storage power service. Learn more about each of them [here](https://venus.filecoin.io/cs/).

## Related Components

Venus loosely describes a collection of components that work together to realize a fully featured Filecoin implementation. List of stand-alone venus modules repos can be found [here](https://venus.filecoin.io/cs/#introducing-venus-components), each assuming different roles in the functioning of Filecoin.

## Contribute

Venus is a universally open project and welcomes contributions of all kinds: code, docs, and more. However, before making a contribution, we ask you to heed these recommendations:

1. If the proposal entails a protocol change, please first submit a [Filecoin Improvement Proposal](https://github.com/filecoin-project/FIPs).
2. If the change is complex and requires prior discussion, [open an issue](https://github.com/ipfs-force-community/venus-cluster/issues) or a [discussion](https://github.com/ipfs-force-community/venus-cluster/discussions) to request feedback before you start working on a pull request. This is to avoid disappointment and sunk costs, in case the change is not actually needed or accepted.

## Community

For general help in using `venus-cluster`, please refer to the [documentation](https://github.com/ipfs-force-community/venus-cluster/tree/main/docs) hosted within the repo. For additional help, you can use one of these channels to ask a question:

- [Slack](https://filecoinproject.slack.com/archives/CEHHJNJS3) (For live discussion with the Community)
- [GitHub](https://github.com/ipfs-force-community/venus-cluster/issues) (Feature/Bug reports, Contributions)
- [Twitter](https://twitter.com/venus_filecoin) (Get the news fast)
- [Meetups](https://venushub.io/meetup/) (A monthly online meetup about Venus in general)
- [I'm feeling lucky](https://github.com/ipfs-force-community/venus-cluster/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) (Pick up a good first issue now!)
