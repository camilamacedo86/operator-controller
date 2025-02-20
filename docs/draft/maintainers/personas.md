## Personas and Roles in OLM

To map the **personas** and **roles** interacting with **OLM**, the following diagrams were created.

The personas are grouped into:
- **Consumers** – Users who consume or interact with the content which is managed by OLM.
- **Producers** – Users who produces content for OLM which might be the cluster extensions or catalogs.

## Consumers

```mermaid
graph LR;

    %% Consumers Section
    subgraph Consumers ["Consumers"]
        CA["Cluster Admin"] 
        EA["Extension Admin"]
    end

    %% Cluster Admin Subgraph
    subgraph ClusterAdmin ["Cluster Admin"]
        CA -->|May serve as| CM["Cluster Monitor"]
        CA -->|May serve as| CCA["Cluster Catalog Admin"]
        CA --> CA1["Set policies & manage clusters"]
        CA --> CA2["Block cluster decommission"]
        CA --> CA3["Monitor performance"]
        CA --> CA4["Manage & distribute catalogs"]
        CA --> CA5["Control registry & add/remove catalogs"]
    end

    %% Cluster Monitor Subgraph
    subgraph ClusterMonitor ["Cluster Monitor"]
        CM --> CM1["Platform monitoring"]
        CM --> CM2["Review & validate extensions"]
        CM --> CM3["Notification of administrative needs"]
        CM --> CM4["Monitor performance & receive notifications"]
    end

    %% Cluster Catalog Admin Subgraph
    subgraph ClusterCatalogAdmin ["Cluster Catalog Admin"]
        CCA --> CCA1["Manage catalogs"]
        CCA --> CCA2["Distribute catalogs"]
        CCA --> CCA3["Control registry & add/remove catalogs"]
        CCA --> CCA4["Browse & validate catalogs"]
        CCA --> CCA5["Review health of extensions"]
    end

    %% Extension Admin Subgraph
    subgraph ExtensionAdmin ["Extension Admin"]
        EA --> EA1["Create & manage extensions"]
        EA --> EA2["Grant access & approve versions"]
        EA --> EA3["Browse & validate catalogs"]
        EA --> EA4["Review health of extensions"]
    end

    %% Styling
    classDef section fill:#EAEAEA,stroke:#000,stroke-width:1px;
    classDef graybox fill:#D3D3D3,stroke:#000,stroke-width:1px;
    classDef darkblue fill:#003366,color:#FFFFFF,stroke:#000,stroke-width:1px;
    classDef lightblue fill:#99CCFF,color:#000000,stroke:#000,stroke-width:1px;

    %% Applying Styles
    class Consumers section;
    class ClusterAdmin,ClusterMonitor,ClusterCatalogAdmin,ExtensionAdmin graybox;
    class CA,EA darkblue;
    class CM,CCA lightblue;
```

---

## Producers

```mermaid
graph LR;

    %% Producers Section
    subgraph Producers ["Producers"]
        EAU["Extension Author"]
        CA["Catalog Admin"]
    end

    %% Catalog Admin Subgraph
    subgraph CatalogAdmin ["Catalog Admin"]
        CA -->|May serve as| CC["Contributor Curator"]
        CA -->|May serve as| CCur["Catalog Curator"]
        CA -->|May serve as| CMan["Catalog Manipulator"]
        CA --> CA1["Manage & distribute catalogs"]
        CA --> CA2["Sign & verify catalogs"]
        CA --> CA3["Enforce catalog policies"]
    end

    %% Extension Author Subgraph
    subgraph ExtensionAuthor ["Extension Author "]
        EAU --> EAU1["Create extensions"]
        EAU --> EAU2["Validate signing"]
        EAU --> EAU3["Automate releases"]
        EAU --> EAU4["Write deployment scripts"]
        EAU --> EAU5["Test & provide images"]
        EAU --> EAU6["Curate & publish bundles"]
        EAU --> EAU7["Manage workflows"]
    end

    %% Contributor Curator Subgraph
    subgraph ContributorCurator ["Contributor Curator "]
        CC --> CC1["Curate content bundles"]
        CC --> CC2["Manage static workflows"]
    end

    %% Catalog Curator Subgraph
    subgraph CatalogCurator ["Catalog Curator "]
        CCur --> CCur1["Sign catalogs"]
        CCur --> CCur2["Manage catalog visibility"]
        CCur --> CCur3["Ensure compliance policies"]
    end

    %% Catalog Manipulator Subgraph
    subgraph CatalogManipulator ["Catalog Manipulator "]
        CMan --> CMan1["Modify & update catalogs"]
        CMan --> CMan2["Enforce metadata policies"]
    end

    %% Styling
    classDef section fill:#EAEAEA,stroke:#000,stroke-width:1px;
    classDef graybox fill:#D3D3D3,stroke:#000,stroke-width:1px;
    classDef darkblue fill:#003366,color:#FFFFFF,stroke:#000,stroke-width:1px;
    classDef lightblue fill:#99CCFF,color:#000000,stroke:#000,stroke-width:1px;

    %% Applying Styles
    class Producers section;
    class CatalogAdmin,ExtensionAuthor,ContributorCurator,CatalogCurator,CatalogManipulator graybox;
    class CA,EAU darkblue;
    class CC,CCur,CMan lightblue;
```
