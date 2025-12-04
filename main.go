package main

import (
    "context"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "maps"
    "os"
    "os/exec"
    "runtime"
    "slices"
    "strconv"

    "github.com/1password/onepassword-sdk-go"
    "github.com/RNCryptor/RNCryptor-go"

    "tableplus-connections/ui"
)

func main() {
    var all bool
    var outputFile string
    var password string
    var open bool
    const allUsage = "Export all connections, without interactive input"
    const outputUsage = "Output filename"
    const passwordUsage = "Export password"
    const openUsage = "Open the export immediately"

    flag.StringVar(&outputFile, "output", "export", outputUsage)
    flag.StringVar(&outputFile, "o", "export", outputUsage + " (shorthand)")

    flag.StringVar(&password, "password", "password", passwordUsage)
    flag.StringVar(&password, "p", "password", passwordUsage + " (shorthand)")

    flag.BoolVar(&all, "all", false, allUsage)
    flag.BoolVar(&open, "open", false, openUsage)
    flag.Parse()

    items, vaults, err := getDatabaseItems()

    if (err != nil) {
        panic(err);
    }

    connections, groups, err := parseAvailableConnections(items, vaults)

    if (err != nil) {
        panic(err);
    }

    var exportable []*AvailableConnection

    if all {
        exportable = connections
    } else {
        var selectable []ui.Item

        for _, connection := range connections {
            selectable = append(selectable, ui.Item{
                ID: connection.ID,
                TitleText: connection.Name,
                DescriptionText: connection.Address,
                GroupID: connection.GroupID,
                Selected: true,
            })
        }

        selected, err := ui.Run(selectable, groups)

        if err != nil {
            if errors.Is(err, ui.ErrAborted) {
                fmt.Println(err.Error())
                os.Exit(1)
            }

            panic(err)
        }

        var selectedIds []string

        for _, item := range selected {
            selectedIds = append(selectedIds, item.ID)
        }

        for _, connection := range connections {
            if slices.Contains(selectedIds, connection.ID) {
                exportable = append(exportable, connection)
            }
        }
    }

    out := convertConnections(exportable)

    jsonString, err := json.MarshalIndent(out, "", "  ")
    if (err != nil) {
        panic(err)
    }

    encrypted, err := rncryptor.Encrypt(password, jsonString)

    if (err != nil) {
        panic(err)
    }

    err = os.WriteFile(outputFile + ".tableplusconnection", encrypted, 0666)

    if (err != nil) {
        panic(err)
    }

    if open {
        fmt.Println("Opening")

        err = openWithApp("TablePlus", outputFile + ".tableplusconnection")

        if (err != nil) {
            panic(err)
        }
    } else {
        fmt.Println("Exported")
    }
}

func getDatabaseItems() ([]*onepassword.Item, []*onepassword.Vault, error) {
    accountName := flag.Arg(0);

    if (accountName == "") {
        return nil, nil, errors.New("Account name is required as the first argument")
    }

    client, err := onepassword.NewClient(
        context.Background(),
        onepassword.WithDesktopAppIntegration(accountName),
        onepassword.WithIntegrationInfo("TablePlus connections", "v0.1.0"),
    )

    if err != nil {
        return nil, nil, err
    }

    vaultOverviews, err := client.Vaults().List(context.Background())

    if err != nil {
        return nil, nil, err
    }

    var databaseItems []*onepassword.Item
    vaults := make(map[string]*onepassword.Vault)

    for _, vault := range vaultOverviews {
        itemOverviews, err := client.Items().List(context.Background(), vault.ID)

        if err != nil {
            return nil, nil, err
        }

        var vaultDatabaseItemOverviewIds []string

        for _, itemOverview := range itemOverviews {
            if itemOverview.Category == onepassword.ItemCategoryDatabase {
                vaultDatabaseItemOverviewIds = append(vaultDatabaseItemOverviewIds, itemOverview.ID)

                if _, contains := vaults[itemOverview.VaultID]; !contains {
                    actualVault, err := client.Vaults().Get(context.Background(), itemOverview.VaultID, onepassword.VaultGetParams{});

                    if err != nil {
                        panic(err)
                    }

                    vaults[itemOverview.VaultID] = &actualVault
                }
            }
        }

        items, err := client.Items().GetAll(context.Background(), vault.ID, vaultDatabaseItemOverviewIds)

        for _, item := range items.IndividualResponses {
            if (item.Error != nil) {
                return nil, nil, errors.New(string(item.Error.Internal()))
            }

            databaseItems = append(databaseItems, item.Content)
        }
    }

    return databaseItems, slices.Collect(maps.Values(vaults)), nil
}

func parseAvailableConnections(items []*onepassword.Item, vaults []*onepassword.Vault) ([]*AvailableConnection, []*ui.Group, error) {
    var availableConnections []*AvailableConnection
    var groups []*ui.Group

    for _, vault := range vaults {
        groups = append(groups, &ui.Group{
            ID:          vault.ID,
            Name:        vault.Title,
            Description: "Idk",
        })
    }

    for _, item := range items {
        var address *string
        var port *int
        var username *string
        var password *string

        for _, field := range item.Fields {
            if (field.ID == "hostname" && address == nil) {
                address = &field.Value
            }

            if (field.ID == "port" && port == nil) {
                portInt, err := strconv.Atoi(field.Value)

                if (err == nil) {
                    port = &portInt
                }
            }

            if (field.ID == "username" && username == nil) {
                username = &field.Value
            }

            if (field.ID == "password" && password == nil) {
                password = &field.Value
            }
        }

        if (address == nil || port == nil || username == nil || password == nil) {
            continue
        }

        availableConnections = append(availableConnections, &AvailableConnection{
            ID: item.ID,
            GroupID: item.VaultID,
            Name: item.Title,
            Address: *address,
            Port: *port,
            Username: *username,
            Password: *password,
        })
    }

    return availableConnections, groups, nil
}

func convertConnections(in []*AvailableConnection) []*OutputConnection {
    out := make([]*OutputConnection, 0, len(in))

    for _, c := range in {
        out = append(out, &OutputConnection{
            DatabaseUser:     c.Username,
            ServerAddress:    c.Address,
            DatabaseHost:     c.Address,
            ConnectionName:   c.Name,
            DatabasePassword: c.Password,
            DatabasePort:     fmt.Sprintf("%d", c.Port),

            // Defaults that match your sample JSON, TODO: fix
            Driver:              "PostgreSQL",
            Enviroment:          "local",
            StatusColor:         "#007F3D",
            ServerPort:          "22",
            TlsKeyName:          "Key...,Cert...,CA Cert...",
            TlsKeyPaths:         []string{"", "", ""},
            ServerPrivateKeyName:"Import a private key...",
            RecentlyOpened:      []string{},
            OtherOptions:        []string{},
            RecentlySchema:      []string{},
            RecentUsedBackupOptions: []string{},
            RecentUsedRestoreOptions: []string{},
            SectionStates:       map[string]any{},
            Favorites:           map[string]any{},
        })
    }

    return out
}

func openWithApp(app, target string) error {
    if runtime.GOOS != "darwin" {
        return fmt.Errorf("openWithApp is only implemented for macOS")
    }

    cmd := exec.Command("open", "-a", app, target)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    return cmd.Run()
}

type AvailableConnection struct {
    ID string
    GroupID string
    Name string
    Address string
    Port int
    Username string
    Password string
}

type OutputConnection struct {
    DatabaseType              string                 `json:"DatabaseType"`
    TlsKeyName                string                 `json:"TlsKeyName"`
    IsUsePrivateKey           int                    `json:"isUsePrivateKey"`
    LimitQueryRowsReturned    int                    `json:"LimitQueryRowsReturned"`
    StartupCommands           string                 `json:"StartupCommands"`
    RecentlyOpened            []string               `json:"RecentlyOpened"`
    DatabaseSocket            string                 `json:"DatabaseSocket"`
    DatabaseUser              string                 `json:"DatabaseUser"`
    ServerAddress             string                 `json:"ServerAddress"`
    TlsKeyPaths               []string               `json:"TlsKeyPaths"`
    StatusColor               string                 `json:"statusColor"`
    DatabaseEncoding          string                 `json:"DatabaseEncoding"`
    ServerUser                string                 `json:"ServerUser"`
    RecentUsedBackupOptions   []string               `json:"RecentUsedBackupOptions"`
    ShowSystemSchemas         int                    `json:"ShowSystemSchemas"`
    Enviroment                string                 `json:"Enviroment"`
    DatabasePath              string                 `json:"DatabasePath"`
    DriverVersion             int                    `json:"DriverVersion"`
    Driver                    string                 `json:"Driver"`
    AdvancedSafeModeLevel     int                    `json:"AdvancedSafeModeLevel"`
    HideFunctionSection       int                    `json:"HideFunctionSection"`
    LimitRowsReturned         int                    `json:"LimitRowsReturned"`
    ConnectionName            string                 `json:"ConnectionName"`
    DatabaseWarehouse         string                 `json:"DatabaseWarehouse"`
    OtherOptions              []string               `json:"OtherOptions"`
    ServerPasswordMode        int                    `json:"ServerPasswordMode"`
    IsUseSocket               int                    `json:"isUseSocket"`
    TLSMode                   int                    `json:"tLSMode"`
    ShowRecentlySection       int                    `json:"ShowRecentlySection"`
    SectionStates             map[string]any         `json:"SectionStates"`
    Favorites                 map[string]any         `json:"Favorites"`
    IsOverSSH                 int                    `json:"isOverSSH"`
    ServerPassword            string                 `json:"ServerPassword"`
    ServerPort                string                 `json:"ServerPort"`
    DatabasePort              string                 `json:"DatabasePort"`
    DatabaseHost              string                 `json:"DatabaseHost"`
    DatabaseName              string                 `json:"DatabaseName"`
    RecentlySchema            []string               `json:"RecentlySchema"`
    RecentUsedBackupDriverName string                `json:"RecentUsedBackupDriverName"`
    RecentUsedBackupGzip      int                    `json:"RecentUsedBackupGzip"`
    RecentUsedRestoreOptions  []string               `json:"RecentUsedRestoreOptions"`
    Authenticator             string                 `json:"Authenticator"`
    DatabaseUserRole          string                 `json:"DatabaseUserRole"`
    DatabasePassword          string                 `json:"DatabasePassword"`
    DatabasePasswordMode      int                    `json:"DatabasePasswordMode"`
    ServerPrivateKeyName      string                 `json:"ServerPrivateKeyName"`
    DatabaseKeyPassword       string                 `json:"DatabaseKeyPassword"`
    SafeModeLevel             int                    `json:"SafeModeLevel"`
    ReadIntentOnly            int                    `json:"ReadIntentOnly"`
}
